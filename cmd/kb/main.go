package main

import (
	"context"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/web"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	if command == "check" {
		c := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
		r, e := c.Get("http://127.0.0.1:8080/readyz")
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("not ready: %d", r.StatusCode)
		}
		return nil
	}
	if command != "serve" {
		return fmt.Errorf("unknown command %q", command)
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	lock, e := storage.AcquireInstanceLock(cfg.DataDir)
	if e != nil {
		return e
	}
	defer lock.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	store, e := storage.Open(ctx, cfg.DataDir)
	if e != nil {
		return e
	}
	defer store.Close()
	ln, e := net.Listen("tcp", cfg.ListenAddr)
	if e != nil {
		return e
	}
	log.Printf("listening on %s", ln.Addr())
	return web.Serve(ctx, ln, web.HealthRoutes(store))
}
