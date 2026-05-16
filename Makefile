BINARY := lockie
MODULE := github.com/ujjalsharma100/lockie

VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/version.Date=$(DATE)'

.PHONY: build test lint tidy

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/lockie

test:
	go test ./... -race -count=1

lint:
	golangci-lint run --timeout 5m

tidy:
	go mod tidy
