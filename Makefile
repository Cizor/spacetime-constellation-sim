# Simple Makefile for spacetime-constellation-sim

# Go toolchain configuration
GO       ?= go
GOMOD    ?= ./...
SIM_CMD  ?= ./cmd/simulator

# Optional: override these via env if you want
GOFLAGS  ?=
RACEFLAGS ?= -race

.PHONY: all build run-sim test race lint protos run-nbi clean

## Default: build simulator
all: build

## Build the simulator CLI (Scope 1–2)
build:
	$(GO) build $(GOFLAGS) $(SIM_CMD)

## Run the simulator demo
run-sim:
	$(GO) run $(GOFLAGS) $(SIM_CMD)

## Run all tests
test:
	$(GO) test $(GOFLAGS) ./...

## Run tests with race detector
race:
	$(GO) test $(GOFLAGS) $(RACEFLAGS) ./...

## Lint (requires golangci-lint installed in PATH)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install from https://golangci-lint.run/ then re-run 'make lint'."; \
		exit 1; \
	}
	golangci-lint run ./...

## Generate protobuf / gRPC code for Scope 3 (future)
## This assumes you'll add scripts/gen_protos.sh and third_party/aalyria later.
protos:
	@echo "Generating protobuf code (Scope 3)…"
	@./scripts/gen_protos.sh

## Run the NBI gRPC server (Scope 3, future cmd/nbi-server)
run-nbi:
	$(GO) run $(GOFLAGS) ./cmd/nbi-server

## Clean build artifacts (Go is mostly clean by default, but you can extend this)
clean:
	@echo "Nothing special to clean (Go build cache is managed by 'go clean')."
	@echo "Run 'go clean ./...' if you want to clear build/test caches."