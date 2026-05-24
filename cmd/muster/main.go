package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	api "github.com/gitrgoliveira/muster/internal/api"
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/config"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/jsonl"
	"github.com/gitrgoliveira/muster/internal/ws"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "Usage: muster serve [--addr HOST:PORT] [--beads-dir DIR] [--bd-bin PATH]\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7766", "listen address")
	beadsDirFlag := fs.String("beads-dir", "", "path to beads directory (overrides $BEADS_DIR)")
	bdBinFlag := fs.String("bd-bin", "", "path to bd binary (overrides $BD_BIN and PATH)")
	fs.Parse(os.Args[2:]) //nolint:errcheck // ExitOnError handles the error

	// Validate addr format.
	if _, _, err := net.SplitHostPort(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "invalid addr %q: %v\n", *addr, err)
		os.Exit(1)
	}

	// Resolve beads directory.
	beadsDir, err := config.ResolveBeadsDir(*beadsDirFlag, os.Getenv("BEADS_DIR"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Load backend config from metadata.json.
	cfg, err := config.LoadBackendConfig(beadsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Resolve bd binary path.
	bdBin := *bdBinFlag
	if bdBin == "" {
		bdBin = os.Getenv("BD_BIN")
	}
	cfg.BdBin = bdBin

	beadsVersion := "0.9.1"

	hub := ws.NewHub(beadsVersion)
	go hub.Run()

	// Construct the read backend.
	var backend store.Backend
	switch cfg.Mode {
	case "embedded":
		b, berr := jsonl.NewJSONL(beadsDir)
		if berr != nil {
			fmt.Fprintf(os.Stderr, "cannot open issues.jsonl: %v\n", berr)
			os.Exit(1)
		}
		backend = b
	default:
		// Fall back to in-memory seed for modes not yet wired (e.g. "remote").
		backend = store.NewMemoryBackend(store.SeedIssues())
	}

	svc := services.NewBeadService(backend, nil, hub.Broadcast)

	// Print startup banner.
	bdCLIDisplay := cfg.BdBin
	if bdCLIDisplay == "" {
		bdCLIDisplay = "(missing)"
	}
	fmt.Printf(
		"muster listening on http://%s\n  beadsDir=%s doltDatabase=%s doltMode=%s readSource=%s bdCLI=%s schemaVersion=%d\n",
		*addr, cfg.BeadsDir, cfg.DoltDatabase, cfg.Mode, cfg.ReadSource, bdCLIDisplay, cfg.SchemaVersion,
	)

	// Start file watcher for embedded mode.
	watcherOut := make(chan store.WatcherEvent, 32)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Mode == "embedded" {
		jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
		w := store.NewWatcher(backend, jsonlPath, watcherOut)
		go w.Run(ctx) //nolint:errcheck

		// Fan watcher events into the WS hub.
		go func() {
			for {
				select {
				case ev := <-watcherOut:
					for _, id := range ev.CreatedIDs {
						hub.Broadcast(ws.Frame{Type: ws.EventBeadCreated, ID: id})
					}
					for _, id := range ev.ChangedIDs {
						hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated, ID: id})
					}
					for _, id := range ev.DeletedIDs {
						hub.Broadcast(ws.Frame{Type: ws.EventBeadDeleted, ID: id})
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	statusCfg := health.StatusConfig{
		BeadsVersion:  beadsVersion,
		BeadsDir:      cfg.BeadsDir,
		DoltDatabase:  cfg.DoltDatabase,
		DoltMode:      cfg.Mode,
		ReadSource:    cfg.ReadSource,
		BdCLI:         cfg.BdBin,
		SchemaVersion: cfg.SchemaVersion,
		Pinger:        backend,
	}

	handler := api.NewRouter(svc, hub, UI, statusCfg)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

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

	backend.Close() //nolint:errcheck
}
