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
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/store/dolt"
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

	// Resolve bd CLI binary (optional).
	var cli *bdshell.CLI
	if cfg.BdBin != "" {
		var cliErr error
		cli, cliErr = bdshell.NewCLI(cfg.BdBin, beadsDir)
		if cliErr != nil {
			fmt.Fprintf(os.Stderr, "warning: bd CLI not available: %v\n", cliErr)
			cli = nil
		}
	}

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
	case "remote":
		// Start the dolt server (idempotent).
		if cli != nil {
			startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := cli.DoltStart(startCtx); err != nil {
				fmt.Fprintf(os.Stderr, "warning: bd dolt start: %v\n", err)
			}
			startCancel()
		}
		dsn := buildDoltDSN(cfg)
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		b, berr := dolt.NewDolt(initCtx, dsn)
		initCancel()
		if berr != nil {
			fmt.Fprintf(os.Stderr, "cannot connect to dolt server: %v\n", berr)
			os.Exit(1)
		}
		backend = b
	default:
		// Fallback — should not happen in practice after config validation.
		backend = store.NewMemoryBackend(store.SeedIssues())
	}

	var svcCLI services.CLIRunner
	if cli != nil {
		svcCLI = cli
	}
	svc := services.NewBeadService(backend, svcCLI, hub.Broadcast)

	// Print startup banner.
	bdCLIDisplay := "(missing — write endpoints disabled)"
	if cli != nil {
		bdCLIDisplay = cli.Path
	}
	fmt.Printf("muster listening on http://%s\n", *addr)
	fmt.Printf("  build         = dev\n")
	fmt.Printf("  schemaVersion = %d\n", cfg.SchemaVersion)
	fmt.Printf("  beadsDir      = %s\n", cfg.BeadsDir)
	fmt.Printf("  doltDatabase  = %s\n", cfg.DoltDatabase)
	fmt.Printf("  doltMode      = %s\n", cfg.Mode)
	fmt.Printf("  readSource    = %s\n", cfg.ReadSource)
	fmt.Printf("  bdCLI         = %s\n", bdCLIDisplay)

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

// buildDoltDSN constructs a MySQL DSN for Dolt from BackendConfig.
func buildDoltDSN(cfg config.BackendConfig) string {
	host := cfg.DoltHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.DoltPort
	if port == 0 {
		port = 3306
	}
	user := cfg.DoltUser
	if user == "" {
		user = "root"
	}
	password := cfg.DoltPassword
	if password != "" {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&collation=utf8mb4_0900_ai_ci",
			user, password, host, port, cfg.DoltDatabase)
	}
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?parseTime=true&collation=utf8mb4_0900_ai_ci",
		user, host, port, cfg.DoltDatabase)
}
