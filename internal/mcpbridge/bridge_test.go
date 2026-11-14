package mcpbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/mcpbridge"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testKey = "kb_mcp_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

func TestBridgeURLValidation(t *testing.T) {
	for _, endpoint := range []string{"", "http://kb.example/mcp", "https://user:secret@kb.example/mcp", "https://kb.example/mcp?token=secret", "https://kb.example/mcp#secret", "file:///tmp/socket", "https://"} {
		if err := mcpbridge.ValidateConfig(endpoint, testKey); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("bad URL accepted or leaked: %q %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"https://kb.example/mcp", "http://127.0.0.1:8080/mcp", "http://[::1]:8080/mcp", "http://localhost:8080/mcp"} {
		if err := mcpbridge.ValidateConfig(endpoint, testKey); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"", "secret\r\nOther: evil", strings.Repeat("x", 50)} {
		if err := mcpbridge.ValidateConfig("https://kb.example/mcp", key); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
}

func TestBridgeForwardsToolsAndEOF(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "remote", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "Read text", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(_ context.Context, _ *mcp.CallToolRequest, in struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, struct {
		Text string `json:"text"`
	}, error) {
		return nil, in, nil
	})
	remote := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "denied", 401)
			return
		}
		remote.ServeHTTP(w, r)
	}))
	defer srv.Close()
	inputR, inputW := io.Pipe()
	outputR, outputW := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Run(ctx, srv.URL, testKey, inputR, outputW) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "local", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: outputR, Writer: inputW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 || !tools.Tools[0].Annotations.ReadOnlyHint {
		t.Fatalf("tools: %v %v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "Привет 🚀"}})
	if err != nil || result.IsError || result.StructuredContent == nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "Привет 🚀") {
		t.Fatalf("forward: %+v %v", result, err)
	}
	session.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("adapter hung on EOF")
	}
}

func TestBridgeRejectsRedirectWithoutForwardingKey(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(500) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	err := mcpbridge.Run(context.Background(), source.URL, testKey, io.NopCloser(strings.NewReader("")), discardWriter{})
	if err == nil || called || strings.Contains(err.Error(), testKey) {
		t.Fatalf("unsafe redirect: %v called=%v", err, called)
	}
}

func TestBridgeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mcpbridge.Run(ctx, "http://127.0.0.1:1/mcp", testKey, io.NopCloser(strings.NewReader("")), discardWriter{})
	if err == nil {
		t.Fatal("cancelled connection accepted")
	}
}

func TestBridgeCancellationStopsAnActiveCall(t *testing.T) {
	for _, eof := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "EOF"}[eof], func(t *testing.T) { testBridgeStopActiveCall(t, eof) })
	}
}

func testBridgeStopActiveCall(t *testing.T, eof bool) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server := mcp.NewServer(&mcp.Implementation{Name: "remote", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "wait"}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return nil, in, ctx.Err()
		case <-release:
			return nil, in, nil
		}
	})
	srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true}))
	defer srv.Close()
	inputR, inputW := io.Pipe()
	outputR, outputW := io.Pipe()
	defer inputR.Close()
	defer inputW.Close()
	defer outputR.Close()
	defer outputW.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Run(ctx, srv.URL, testKey, inputR, outputW) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "local", Version: "1.0.0"}, nil)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connectCancel()
	session, err := client.Connect(connectCtx, &mcp.IOTransport{Reader: outputR, Writer: inputW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		session.CallTool(connectCtx, &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{}})
	}()
	select {
	case <-entered:
	case <-connectCtx.Done():
		t.Fatal("remote call did not start")
	}
	if eof {
		inputW.Close()
	} else {
		cancel()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("adapter did not stop after cancellation during an active call")
	}
	select {
	case <-callDone:
	case <-time.After(2 * time.Second):
		t.Fatal("local call remained blocked after adapter shutdown")
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) Close() error                { return nil }

func TestBridgeUnknownCreateOutcomeDoesNotEncourageBlindRetry(t *testing.T) {
	var saved atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "remote", Version: "1.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "create_article"}, func(_ context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct {
		DraftID string `json:"draft_id"`
	}, error) { saved.Add(1); return nil, struct {
		DraftID string `json:"draft_id"`
	}{"saved-draft"}, nil })
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var message struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &message) == nil && message.Method == "tools/call" && message.Params.Name == "create_article" {
			// Complete the write, then lose the response before it reaches the client.
			transport.ServeHTTP(httptest.NewRecorder(), r)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		transport.ServeHTTP(w, r)
	}))
	defer srv.Close()
	inputR, inputW := io.Pipe()
	outputR, outputW := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Run(ctx, srv.URL, testKey, inputR, outputW) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "local", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: outputR, Writer: inputW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: map[string]any{}})
	if err != nil || !result.IsError || saved.Load() != 1 {
		t.Fatalf("ambiguous write setup: saved=%d err=%v", saved.Load(), err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Мои черновики") || strings.Contains(text, "повторите запрос") {
		t.Fatalf("unsafe write guidance: %s", text)
	}
	session.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("bridge did not stop")
	}
}
