# cmdctx Makefile
# Primary targets: build, install, uninstall, test, lint, clean

BINARY     := cmdctx
MODULE     := github.com/slim/cmdctx
CMD_DIR    := ./cmd/cmdctx
BUILD_DIR  := ./bin
INSTALL_DIR ?= $(HOME)/.local/bin
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE) -s -w"

.PHONY: all build install uninstall clean test lint vet fmt check doctor help

all: build

## build: Compile the binary to ./bin/cmdctx
build:
	@mkdir -p $(BUILD_DIR)
	@echo "▶ Building $(BINARY) $(VERSION)..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY)"

## install: Install binary to ~/.local/bin (override with INSTALL_DIR=/usr/local/bin make install)
install: build
	@mkdir -p $(INSTALL_DIR)
	@echo "▶ Installing to $(INSTALL_DIR)/$(BINARY)..."
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@chmod 755 $(INSTALL_DIR)/$(BINARY)
	@echo "✓ Installed: $(INSTALL_DIR)/$(BINARY)"
	@if ! echo "$$PATH" | grep -q "$(INSTALL_DIR)"; then \
		echo ""; \
		echo "⚠  $(INSTALL_DIR) is not in your PATH."; \
		echo "   Add this to your shell profile:"; \
		echo "     export PATH=\"$$HOME/.local/bin:$$PATH\""; \
		echo ""; \
	fi

## install-global: Install to /usr/local/bin (requires sudo)
install-global: build
	@echo "▶ Installing to /usr/local/bin/$(BINARY) (requires sudo)..."
	@sudo cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@sudo chmod 755 /usr/local/bin/$(BINARY)
	@echo "✓ Installed globally: /usr/local/bin/$(BINARY)"

## uninstall: Remove installed binary (does NOT remove app data)
uninstall:
	@echo "▶ Removing $(INSTALL_DIR)/$(BINARY)..."
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "✓ Uninstalled"
	@echo "  Note: App data in ~/.cmdctx was NOT removed."
	@echo "  To remove app data: rm -rf ~/.cmdctx"

## uninstall-all: Remove binary AND all app data (destructive!)
uninstall-all: uninstall
	@echo "⚠  Removing all app data: ~/.cmdctx"
	@rm -rf $(HOME)/.cmdctx
	@echo "✓ All data removed"

## test: Run all tests
test:
	@echo "▶ Running tests..."
	go test -v -race -count=1 ./...

## test-short: Run tests without integration tests
test-short:
	@echo "▶ Running short tests..."
	go test -short -v -count=1 ./...

## cover: Run tests with coverage
cover:
	@echo "▶ Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

## lint: Run golangci-lint (install it separately)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, running go vet instead"; \
		go vet ./...; \
	fi

## vet: Run go vet
vet:
	go vet ./...

## fmt: Format all Go files
fmt:
	gofmt -w .

## check: Run fmt, vet, and tests
check: fmt vet test

## clean: Remove build artifacts
clean:
	@rm -rf $(BUILD_DIR) coverage.out coverage.html
	@echo "✓ Cleaned"

## doctor: Run cmdctx doctor (after building)
doctor: build
	$(BUILD_DIR)/$(BINARY) doctor

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
