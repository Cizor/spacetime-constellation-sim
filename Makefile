# Simple Makefile for spacetime-constellation-sim

# Go toolchain configuration
GO        ?= go
GOMOD     ?= ./...
SIM_CMD   ?= ./cmd/simulator

# Optional: override these via env if you want
GOFLAGS   ?=
RACEFLAGS ?= -race

# --- Proto / Aalyria NBI code generation ---

# Root where vendored Aalyria protos live
PROTO_API_ROOT    := third_party/aalyria
# Root where googleapis protos are cloned (local-only dependency)
PROTO_GOOGLE_ROOT := third_party/googleapis
# Where generated Go code will be written
PROTO_OUT_DIR     := internal/genproto/aalyria

# Directories under api/ that we care about
PROTO_SRC_DIRS := \
  $(PROTO_API_ROOT)/api/common \
  $(PROTO_API_ROOT)/api/nbi/v1alpha \
  $(PROTO_API_ROOT)/api/nbi/v1alpha/resources \
  $(PROTO_API_ROOT)/api/types

# All .proto files in those directories
PROTO_FILES := $(foreach dir,$(PROTO_SRC_DIRS),$(wildcard $(dir)/*.proto))

# Convert to paths relative to PROTO_API_ROOT so protoc writes
# files under $(PROTO_OUT_DIR)/api/...
PROTO_FILES_REL := $(patsubst $(PROTO_API_ROOT)/%,%,$(PROTO_FILES))

.PHONY: all build run-sim test race lint proto protos run-nbi clean

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

## Generate protobuf / gRPC code for Aalyria APIs (Scope 3)
proto:
	@if not exist "$(PROTO_OUT_DIR)" mkdir "$(PROTO_OUT_DIR)"
	protoc -I $(PROTO_API_ROOT) -I $(PROTO_GOOGLE_ROOT) \
	  --go_out=$(PROTO_OUT_DIR) --go_opt=paths=source_relative \
	  --go-grpc_out=$(PROTO_OUT_DIR) --go-grpc_opt=paths=source_relative \
	  $(PROTO_FILES_REL)

## Backwards-compatible alias
protos: proto

## Run the NBI gRPC server (Scope 3, future cmd/nbi-server)
run-nbi:
	$(GO) run $(GOFLAGS) ./cmd/nbi-server

## Clean build artifacts (Go is mostly clean by default, but you can extend this)
clean:
	@echo "Nothing special to clean (Go build cache is managed by 'go clean')."
	@echo "Run 'go clean ./...' if you want to clear build/test caches."
