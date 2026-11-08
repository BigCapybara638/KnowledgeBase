package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"github.com/BigCapybara638/KnowledgeBase/internal/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("KB_MCP_TEST_PROCESS") != "1" {
		return
	}
	main()
	os.Exit(0)
}

func TestAdapterProcessAgainstApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := testutil.Store(t)
	id := testutil.User(t, s, "user")
	a, err := auth.New(s, auth.Config{PublicURL: "https://kb.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := a.CreateMCPToken(ctx, id, "Process", 90)
	if err != nil {
		t.Fatal(err)
	}
	app, err := web.New(config.Config{PublicURL: "https://kb.test", DataDir: s.DataDir}, s, a)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app)
	defer srv.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, executable, "-test.run=^TestMCPHelperProcess$")
	command.Env = append(os.Environ(), "KB_MCP_TEST_PROCESS=1", "KB_MCP_URL="+srv.URL+"/mcp", "KB_MCP_TOKEN="+raw)
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "process-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("process connect: %v %s", err, stderr.String())
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("process tools: %v %v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_articles", Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("process call: %+v %v", result, err)
	}
	keys, err := a.ListMCPTokens(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RevokeMCPToken(ctx, id, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_articles", Arguments: map[string]any{}})
	if err == nil && !result.IsError {
		t.Fatal("adapter bypassed key revocation")
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), raw) {
		t.Fatal("process logged secret")
	}
}
