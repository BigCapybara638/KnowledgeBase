package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckUsesConfiguredPort(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("ready"))
	}))
	defer s.Close()
	t.Setenv("KB_LISTEN_ADDR", strings.TrimPrefix(s.URL, "http://"))
	if e := run([]string{"check"}); e != nil {
		t.Fatal(e)
	}
}
