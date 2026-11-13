# parcher — parch (page archiver) and pscribe (browser script runner).
#
# Both commands build into ./bin. `go install ./...` also works, but leaves
# both reporting version "dev": the version is stamped through -ldflags,
# and only this Makefile derives it from git.

BIN     := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

PARCH   := ./parchmint/cmd/parch
PSCRIBE := ./pscription/cmd/pscribe

.PHONY: all build install test vet lint probe cross dist ocr-check clean check help

all: build ## Build both commands (default)

build: ## Build parch and pscribe into ./bin
	@mkdir -p $(BIN)
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/ $(PARCH) $(PSCRIBE)
	@echo "built $(VERSION):" && ls -1 $(BIN)

install: ## Install both commands into GOBIN, version-stamped
	go install -ldflags "$(LDFLAGS)" $(PARCH) $(PSCRIBE)

test: ## Run every test
	go test ./...

vet: ## go vet the whole tree
	go vet ./...

# Run from the module's own toolchain (go run), not from whatever copy is on
# PATH: a staticcheck built with an older Go refuses a newer module outright.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@latest
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@latest

lint: vet ## vet + staticcheck + govulncheck (downloads the tools on first run)
	go run $(STATICCHECK) ./...
	go run $(GOVULNCHECK) ./...

probe: ## Browser-driven probe suite: silent-loss checks on a local page (needs Chrome)
	PARCH_BROWSER_TESTS=1 go test -count=1 -run Probe ./parchmint/capture/

# Linux builds are pure Go (CGO off): OCR there is tesseract, run as a
# program. macOS builds need cgo for Apple Vision, so they are only built
# natively on a Mac; from Linux a macOS binary still compiles, without Vision.
LINUX_TARGETS  := linux/amd64 linux/arm64
DARWIN_TARGETS := darwin/arm64 darwin/amd64

cross: ## Compile every release target (no output kept) — a build check
	@for t in $(LINUX_TARGETS) $(DARWIN_TARGETS); do \
	  echo "compile $$t"; \
	  CGO_ENABLED=0 GOOS=$${t%/*} GOARCH=$${t#*/} go build -o /dev/null $(PARCH) || exit 1; \
	  CGO_ENABLED=0 GOOS=$${t%/*} GOARCH=$${t#*/} go build -o /dev/null $(PSCRIBE) || exit 1; done

dist: ## Release binaries into ./dist/<os>-<arch>/ (macOS ones only when run on a Mac)
	@for t in $(LINUX_TARGETS); do \
	  out=dist/$${t%/*}-$${t#*/}; mkdir -p $$out; echo "$$out"; \
	  CGO_ENABLED=0 GOOS=$${t%/*} GOARCH=$${t#*/} go build -trimpath -ldflags "$(LDFLAGS)" -o $$out/ $(PARCH) $(PSCRIBE) || exit 1; done
	@if [ "$$(uname -s)" = Darwin ]; then for t in $(DARWIN_TARGETS); do \
	  out=dist/$${t%/*}-$${t#*/}; mkdir -p $$out; echo "$$out"; \
	  CGO_ENABLED=1 GOOS=$${t%/*} GOARCH=$${t#*/} go build -trimpath -ldflags "$(LDFLAGS)" -o $$out/ $(PARCH) $(PSCRIBE) || exit 1; done; \
	else echo "skipping macOS binaries: Apple Vision needs cgo, so build them on a Mac"; fi

ocr-check: build ## Tesseract OCR end to end on a local page (needs Chrome and tesseract)
	PARCH=$(BIN)/parch parchmint/scripts/ocr-check.sh

clean: ## Remove ./bin
	rm -rf $(BIN)

check: build ## Reference-site capture check (needs Chrome and .sites.env)
	PARCH=$(BIN)/parch parchmint/scripts/check-archives.sh

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk -F':.*?## ' '{printf "  make %-8s %s\n", $$1, $$2}'
