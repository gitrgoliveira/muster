package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	adapterclaude "github.com/gitrgoliveira/muster/internal/adapter/claude"
	api "github.com/gitrgoliveira/muster/internal/api"
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/config"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/store/dolt"
	"github.com/gitrgoliveira/muster/internal/store/jsonl"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// repoFlags is a flag.Value that supports repeatable --repo flags.
type repoFlags []string

func (r *repoFlags) String() string     { return fmt.Sprintf("%v", *r) }
func (r *repoFlags) Set(v string) error { *r = append(*r, v); return nil }

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "Usage: muster serve [--addr HOST:PORT] [--beads-dir DIR] [--bd-bin PATH]\n")
		fmt.Fprintf(os.Stderr, "       [--repo PREFIX=PATH]... [--worktrees-dir DIR] [--run-timeout DURATION]\n")
		fmt.Fprintf(os.Stderr, "       [--default-permission-mode MODE]\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7766", "listen address")
	beadsDirFlag := fs.String("beads-dir", "", "path to beads directory (overrides $BEADS_DIR)")
	bdBinFlag := fs.String("bd-bin", "", "path to bd binary (overrides $BD_BIN and PATH)")

	// M2 flags.
	var repoFlagList repoFlags
	fs.Var(&repoFlagList, "repo", "repeatable: map bead-ID prefix to repo path (e.g. mp=/path/to/repo)")
	worktreesDirFlag := fs.String("worktrees-dir", "", "directory for per-bead git worktrees (default: ~/.muster/worktrees)")
	// --run-timeout defaults from MUSTER_RUN_TIMEOUT when the flag is not given
	// (parity with the other M2 flags, and with FR-017's documented env
	// fallback). An unparseable env value is warned about and ignored rather
	// than aborting startup.
	runTimeoutDefault := time.Duration(0)
	if v := os.Getenv("MUSTER_RUN_TIMEOUT"); v != "" {
		if d, perr := time.ParseDuration(v); perr == nil {
			runTimeoutDefault = d
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid MUSTER_RUN_TIMEOUT %q: %v (ignoring)\n", v, perr)
		}
	}
	runTimeoutFlag := fs.Duration("run-timeout", runTimeoutDefault, "optional per-run timeout (e.g. 30m); 0 = no timeout (env: MUSTER_RUN_TIMEOUT)")
	defaultPermModeFlag := fs.String("default-permission-mode", os.Getenv("MUSTER_DEFAULT_PERMISSION_MODE"), "default claude permission mode (acceptEdits, dontAsk, etc.)")

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

	// Parse M2 repo map.
	repoMap := config.RepoMap{}
	for _, rv := range repoFlagList {
		if err := config.ParseRepoFlag(repoMap, rv); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	// Also read MUSTER_REPO env.
	if err := config.ParseRepoEnv(repoMap, os.Getenv("MUSTER_REPO")); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Resolve worktrees directory.
	worktreesDir := *worktreesDirFlag
	if worktreesDir == "" {
		worktreesDir = os.Getenv("MUSTER_WORKTREES_DIR")
	}
	if worktreesDir == "" {
		worktreesDir = config.DefaultWorktreesDir()
	}

	// Validate default permission mode if set.
	var defaultPermMode core.PermissionMode
	if *defaultPermModeFlag != "" {
		pm := core.PermissionMode(*defaultPermModeFlag)
		if !pm.Valid() {
			fmt.Fprintf(os.Stderr, "invalid --default-permission-mode %q\n", *defaultPermModeFlag)
			os.Exit(1)
		}
		defaultPermMode = pm
	}

	const beadsVersion = "0.9.1"

	hub := ws.NewHub(beadsVersion)
	go hub.Run()

	// Resolve bd CLI binary (optional — tries LookPath when flag/env is empty).
	var cli *bdshell.CLI
	cli, cliErr := bdshell.NewCLI(cfg.BdBin, beadsDir)
	if cliErr != nil {
		fmt.Fprintf(os.Stderr, "warning: bd CLI not available: %v\n", cliErr)
		cli = nil
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
		dsn := config.BuildDoltDSN(cfg)
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		b, berr := dolt.NewDolt(initCtx, dsn)
		initCancel()
		if berr != nil {
			fmt.Fprintf(os.Stderr, "cannot connect to dolt server: %v\n", berr)
			os.Exit(1)
		}
		backend = b
	default:
		fmt.Fprintf(os.Stderr, "unsupported dolt_mode %q\n", cfg.Mode)
		os.Exit(1)
	}

	// M2: Probe tmux availability.
	var transport tmux.Manager
	var tmuxAvailable bool
	var tmuxVersion string
	realTransport := tmux.NewRealManager("")
	if ver, err := realTransport.Detect(); err == nil {
		transport = realTransport
		tmuxAvailable = true
		tmuxVersion = ver
		fmt.Printf("  tmux            = %s\n", tmuxVersion)
	} else {
		fmt.Printf("  tmux            = not available (%v)\n", err)
		transport = tmux.NewFallbackManager()
	}

	// M2: Probe adapters.
	claudeAdapter := adapterclaude.New(adapterclaude.Options{})
	reg := adapter.NewRegistryWithDefaults(claudeAdapter)

	var adapterInfos []health.AdapterInfo
	detectCtx, detectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	for _, a := range reg.All() {
		result, _ := a.Detect(detectCtx)
		adapterInfos = append(adapterInfos, health.AdapterInfo{
			ID:       string(a.ID()),
			Version:  result.Version,
			LoggedIn: result.LoggedIn,
		})
	}
	detectCancel()

	// onComplete records an agent run's outcome on its bead when the run exits
	// (FR-013/SC-007). M2 limitation: beads has no distinct "review" status
	// (review folds to in_progress per the mapper), so we cannot move the bead
	// to a review column. Instead we append a note describing the outcome and —
	// in remote mode only — broadcast bead.updated so the UI reflects the
	// change. In embedded mode the file watcher already fans the jsonl change
	// into the WS hub; broadcasting here would double-announce. Runs on the
	// orchestrator watcher goroutine — keep it non-blocking-safe.
	doltDB := cfg.DoltDatabase
	isRemoteMode := cfg.Mode == "remote"
	onComplete := func(beadID string, exitCode int, runSucceeded bool) {
		if cli == nil {
			slog.Warn("onComplete: bd CLI unavailable; cannot record run outcome", "bead", beadID)
			return
		}
		var note string
		if runSucceeded {
			note = "muster: agent run completed (exit 0) — awaiting review"
		} else {
			note = fmt.Sprintf("muster: agent run failed (exit %d)", exitCode)
		}
		noteCtx, noteCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer noteCancel()
		iss, err := cli.AppendNote(noteCtx, beadID, note)
		if err != nil {
			slog.Warn("onComplete: failed to record run outcome on bead", "bead", beadID, "err", err)
			return
		}
		if isRemoteMode {
			bead := services.IssueToBead(&iss, doltDB)
			hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated, Bead: &bead})
		}
	}

	// Build orchestrator. config.RepoMap and orchestrator.RepoMap share the
	// identical underlying type (map[string]string), so this is a plain type
	// conversion (no copy).
	orc := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap(repoMap),
		WorktreesDir:    worktreesDir,
		DefaultPermMode: defaultPermMode,
		Publish:         func(f ws.Frame) { hub.Broadcast(f) },
		RunTimeout:      *runTimeoutFlag,
		OnComplete:      onComplete,
	})

	var svcCLI services.CLIRunner
	if cli != nil {
		svcCLI = cli
	}
	// In remote mode no file watcher runs, so the service publishes WS frames
	// directly on write. Embedded mode leaves this off — the watcher is the
	// single WS source there.
	svc := services.NewBeadServiceWithRepo(backend, svcCLI, hub.Broadcast, cfg.DoltDatabase, cfg.Mode == "remote").
		WithOrchestrator(orc.AsServiceDispatcher()).
		WithAttacher(orc.AsSessionAttacher())

	// M2: Recovery scan — re-discover running sessions from before restart.
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	orc.RecoverSessions(recoveryCtx)
	recoveryCancel()

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
	fmt.Printf("  worktreesDir  = %s\n", worktreesDir)
	if len(repoMap) > 0 {
		for k, v := range repoMap {
			fmt.Printf("  repo[%s]     = %s\n", k, v)
		}
	}

	statusCfg := health.StatusConfig{
		BeadsVersion:  beadsVersion,
		BeadsDir:      cfg.BeadsDir,
		ProjectID:     cfg.ProjectID,
		DoltDatabase:  cfg.DoltDatabase,
		DoltMode:      cfg.Mode,
		ReadSource:    cfg.ReadSource,
		BdCLI:         bdCLIDisplay,
		SchemaVersion: cfg.SchemaVersion,
		Pinger:        backend,
		// M2 additions.
		TmuxAvailable: tmuxAvailable,
		TmuxVersion:   tmuxVersion,
		Adapters:      adapterInfos,
		RunCounter:    orc,
	}

	handler := api.NewRouter(svc, hub, UI, statusCfg)

	// Signal context — shared by the embedded-mode watcher below and by the
	// graceful-shutdown wait at the end of main.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the embedded-mode file watcher BEFORE the HTTP server begins
	// accepting requests. In embedded mode, dispatch/completion rely on the
	// watcher to observe the real bd-CLI jsonl write and fan a bead.updated
	// frame into the WS hub (publishOnWrite is off there). If the server
	// accepted a dispatch — or a run completed — in the window before the
	// watcher was running, that write could be missed by connected WS clients.
	watcherOut := make(chan store.WatcherEvent, 32)
	if cfg.Mode == "embedded" {
		jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
		w := store.NewWatcher(backend, jsonlPath, watcherOut)
		go func() {
			if err := w.Run(ctx); err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "watcher: %v\n", err)
			}
		}()

		// Fan watcher events into the WS hub.
		go func() {
			for {
				select {
				case ev := <-watcherOut:
					for _, id := range ev.CreatedIDs {
						bead, err := svc.GetBead(ctx, id)
						if err != nil {
							hub.Broadcast(ws.Frame{Type: ws.EventBeadCreated, ID: id})
							continue
						}
						hub.Broadcast(ws.Frame{Type: ws.EventBeadCreated, Bead: bead})
					}
					for _, id := range ev.ChangedIDs {
						bead, err := svc.GetBead(ctx, id)
						if err != nil {
							hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated, ID: id})
							continue
						}
						hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated, Bead: bead})
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

	// Now that the watcher is running (embedded mode), start accepting requests.
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

	// M2: Graceful shutdown does NOT kill agent tmux sessions (FR-018).
	// Sessions are owned by the user's tmux server and survive restart.
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
		os.Exit(1)
	}

	backend.Close() //nolint:errcheck
}
