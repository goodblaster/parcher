# parcher — parch (page archiver) and pscribe (browser script runner).
#
# Both commands build into ./bin. `go install ./...` also works, but leaves
# parch reporting version "dev": the version is stamped through -ldflags,
# and only this Makefile derives it from git.

BIN     := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

PARCH   := ./parchmint/cmd/parch
PSCRIBE := ./pscription/cmd/pscribe

.PHONY: all build install test vet clean check help

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

clean: ## Remove ./bin
	rm -rf $(BIN)

check: build ## Reference-site capture check (needs Chrome and .sites.env)
	PARCH=$(BIN)/parch parchmint/scripts/check-archives.sh

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk -F':.*?## ' '{printf "  make %-8s %s\n", $$1, $$2}'
