BINARY  := tally
PKG     := .
PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
GO      ?= go
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_STR := tally $(VERSION)
LDFLAGS     := -X 'main.version=$(VERSION_STR)'


.DEFAULT_GOAL := build

.PHONY: all build install uninstall test test-race cover fmt vet lint tidy run clean help

all: build ## Alias for build.

build: ## Build the tally binary.
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install: build ## Build and install the binary to $(BINDIR).
	install -d $(BINDIR)
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)

uninstall: ## Remove the installed binary.
	rm -f $(BINDIR)/$(BINARY)

test: ## Run the test suite.
	$(GO) test ./...

test-race: ## Run the test suite with the race detector.
	$(GO) test -race ./...

cover: ## Run tests and print a coverage summary.
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

fmt: ## Format all Go sources.
	$(GO) fmt ./...

vet: ## Run go vet.
	$(GO) vet ./...

lint: fmt vet ## Format and vet the sources.

tidy: ## Tidy go.mod and go.sum.
	$(GO) mod tidy

run: build ## Build and run tally against the current directory (use ARGS="...").
	./$(BINARY) $(ARGS)

clean: ## Remove build artifacts.
	rm -f $(BINARY) coverage.out

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
