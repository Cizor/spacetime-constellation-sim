.PHONY: all build test

all: build

build:
	go build ./cmd/simulator

test:
	go test ./... -race -v
