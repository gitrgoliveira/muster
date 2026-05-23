BINARY := bin/musterd
PKG    := ./cmd/musterd/

.PHONY: build test run ui-copy cover cover-check lint clean

build:
	go build -o $(BINARY) $(PKG)

test:
	go test -race ./...

run:
	go run $(PKG) serve --addr 127.0.0.1:7766

ui-copy:
	cp -r prototype/. cmd/musterd/ui/
	cp prototype/Muster.html cmd/musterd/ui/index.html

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
