BINARY  := ha-inspector-mcp
PKG     := ./...
BIN_DIR := bin

# The gate. `make check` must be green before any task box is ticked.
.PHONY: check
check: build vet lint test

# -o $(BIN_DIR)/ keeps `go build ./...` from dropping main-package binaries
# into the repository root.
.PHONY: build
build:
	go build -o $(BIN_DIR)/ $(PKG)

.PHONY: vet
vet:
	go vet $(PKG)

# Soft dependency: lint is advisory until golangci-lint is installed
# (`brew install golangci-lint`). It must never block the gate on a fresh clone.
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed — skipping (brew install golangci-lint)"; \
	fi

.PHONY: test
test:
	go test -race $(PKG)

# Integration/contract tests against a real or fixture HA instance.
# Guarded by a build tag so `make check` stays fast and network-free.
.PHONY: test-integration
test-integration:
	go test -tags=integration -count=1 $(PKG)

.PHONY: run
run:
	go run ./cmd/server

# Deployment target is Home Assistant OS on Raspberry Pi (aarch64).
.PHONY: release
release:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
		-o $(BIN_DIR)/$(BINARY)-linux-arm64 ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
		-o $(BIN_DIR)/$(BINARY)-linux-amd64 ./cmd/server

.PHONY: fmt
fmt:
	go fmt $(PKG)

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
	go clean -testcache
