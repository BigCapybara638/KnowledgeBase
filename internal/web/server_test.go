package web

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadinessFailsWithClosedDB(t *testing.T) {
	s, e := storage.Open(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h := HealthRoutes(s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.Close()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatalf("closed DB returned %d", w.Code)
	}
}
func TestShutdownDrainsRequest(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.Write([]byte("saved")) }))
	}()
	result := make(chan int, 1)
	go func() {
		r, e := http.Get("http://" + ln.Addr().String())
		if e != nil {
			result <- 0
			return
		}
		defer r.Body.Close()
		result <- r.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("request was cut short")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if <-result != 200 {
		t.Fatal("request not completed")
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
