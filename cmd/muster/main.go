package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "github.com/gitrgoliveira/muster/internal/api"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "Usage: muster serve [--addr HOST:PORT]\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7766", "listen address")
	fs.Parse(os.Args[2:]) //nolint:errcheck // ExitOnError handles the error

	// Validate addr format.
	if _, _, err := net.SplitHostPort(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "invalid addr %q: %v\n", *addr, err)
		os.Exit(1)
	}

	beadsVersion := "0.9.1"

	hub := ws.NewHub(beadsVersion)
	go hub.Run()

	// Use in-memory backend seeded with prototype issues for now.
	// TODO T044: replace with jsonl.NewJSONL(beadsDir) for embedded mode.
	backend := store.NewMemoryBackend(store.SeedIssues())
	svc := services.NewBeadService(backend, nil, hub.Broadcast)

	handler := api.NewRouter(svc, hub, UI, beadsVersion)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("muster listening on http://%s (build=dev schemaVersion=1)\n", *addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "listen: %v\n", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
		os.Exit(1)
	}
}
