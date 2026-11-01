package web

import (
	"context"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"net"
	"net/http"
	"time"
)

func HealthRoutes(s *storage.Store) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if s.Ready(ctx) != nil {
			http.Error(w, "not ready", 503)
			return
		}
		w.Write([]byte("ready\n"))
	})
	return m
}
func Serve(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan struct{})
	defer close(done)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if srv.Shutdown(c) != nil {
				srv.Close()
			}
		case <-done:
		}
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		<-stopped
		return nil
	}
	return err
}
