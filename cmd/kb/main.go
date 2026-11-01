package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/web"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
		addr := os.Getenv("KB_LISTEN_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return e
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		r, e := c.Get("http://" + net.JoinHostPort(host, port) + "/readyz")
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("not ready: %d", r.StatusCode)
		}
		return nil
	}
	if command == "backup" || command == "restore" || command == "maintenance" {
		return operations(command, args[1:])
	}
	if command == "admin" {
		return bootstrap(args[1:])
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
	defer ln.Close()
	accounts, e := auth.New(store, auth.Config{PublicURL: cfg.PublicURL, Dev: cfg.Dev})
	if e != nil {
		return e
	}
	handler, e := web.New(cfg, store, accounts)
	if e != nil {
		return e
	}
	log.Printf("listening on %s", ln.Addr())
	return web.Serve(ctx, ln, handler)
}

func bootstrap(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("usage: kb admin create --login ... --name ... --password-stdin")
	}
	f := flag.NewFlagSet("admin create", flag.ContinueOnError)
	login := f.String("login", "", "login")
	name := f.String("name", "", "name")
	stdin := f.Bool("password-stdin", false, "read password from stdin")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if !*stdin {
		return fmt.Errorf("--password-stdin is required")
	}
	dir := os.Getenv("KB_DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	lock, e := storage.AcquireInstanceLock(dir)
	if e != nil {
		return e
	}
	defer lock.Close()
	store, e := storage.Open(context.Background(), dir)
	if e != nil {
		return e
	}
	defer store.Close()
	a, e := auth.New(store, auth.Config{})
	if e != nil {
		return e
	}
	b, e := io.ReadAll(io.LimitReader(os.Stdin, 1024))
	if e != nil {
		return e
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	u, e := a.Bootstrap(context.Background(), auth.NewUser{Login: *login, Name: *name, TemporaryPassword: password})
	if e != nil {
		return e
	}
	fmt.Println("Administrator created:", u.Login, "(password change required)")
	return nil
}
