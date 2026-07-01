BINARY := bin/muster
PKG    := ./cmd/muster/

.PHONY: build test test-e2e run ui-copy cover cover-check lint clean help

build:
	go build -o $(BINARY) $(PKG)

test:
	go test -race ./...

run:
	go run $(PKG) serve --addr 127.0.0.1:7766 --beads-dir $${BEADS_DIR:-.beads}

ui-copy:
	cp -r prototype/. cmd/muster/ui/
	cp prototype/Muster.html cmd/muster/ui/index.html

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -func=cover.out

cover-check: cover
	@go tool cover -func=cover.out | awk ' \
	  BEGIN { \
	    thresholds["github.com/gitrgoliveira/muster/internal/core"] = 80; \
	    thresholds["github.com/gitrgoliveira/muster/internal/store"] = 80; \
	    thresholds["github.com/gitrgoliveira/muster/internal/services"] = 80; \
	    thresholds["github.com/gitrgoliveira/muster/internal/ws"] = 75; \
	    thresholds["github.com/gitrgoliveira/muster/internal/api/render"] = 90; \
	    thresholds["github.com/gitrgoliveira/muster/internal/api/middleware"] = 90; \
	    thresholds["github.com/gitrgoliveira/muster/internal/api/beads"] = 70; \
	    thresholds["github.com/gitrgoliveira/muster/internal/api/stream"] = 70; \
	    thresholds["github.com/gitrgoliveira/muster/internal/api/health"] = 70; \
	    thresholds["github.com/gitrgoliveira/muster/internal/adapter"] = 80; \
	    thresholds["github.com/gitrgoliveira/muster/internal/tmux"] = 75; \
	    thresholds["github.com/gitrgoliveira/muster/internal/worktree"] = 85; \
	    thresholds["github.com/gitrgoliveira/muster/internal/orchestrator"] = 80; \
	    fail = 0; \
	  } \
	  /total:/ { \
	    pkg = $$1; sub(":[^:]*$$","",pkg); \
	    pct = $$3; sub(/%/,"",pct); \
	    if (pkg in thresholds && pct+0 < thresholds[pkg]) { \
	      printf "FAIL: %s coverage %.1f%% < required %d%%\n", pkg, pct+0, thresholds[pkg]; \
	      fail = 1; \
	    } \
	  } \
	  END { exit fail }' || (echo "Coverage gate failed"; exit 1)

lint:
	gofmt -l . && go vet ./... && golangci-lint run

clean:
	rm -rf bin/ cover.out

## test-e2e: Run the real end-to-end M2 flow (requires: claude logged in + tmux installed).
##   Uses real claude (Max plan usage, not per-call billing) and real tmux.
##   Skips gracefully if either is unavailable.
##   Timeout is set generously above the test's own 120s wait loop + setup
##   so the harness itself never trips before the test does.
test-e2e:
	go test -tags=e2e -run TestE2E -count=1 -v -timeout 300s ./internal/orchestrator/

help:
	@echo "Available targets:"
	@echo "  build        - Build the muster binary"
	@echo "  test         - Run unit/integration tests (claude always faked; real-tmux tests run if tmux present, else skip)"
	@echo "  test-e2e     - Run real end-to-end M2 test (needs: claude logged in + tmux)"
	@echo "                 Uses Max plan usage allowance. Skips if unavailable."
	@echo "  cover        - Generate and print test coverage"
	@echo "  cover-check  - Run coverage and enforce per-package gates"
	@echo "  run          - Start muster serve locally"
	@echo "  lint         - Run gofmt, go vet, golangci-lint"
	@echo "  clean        - Remove build artifacts"
