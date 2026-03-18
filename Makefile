.PHONY: build test lint install dev-release dist clean

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Version info
VERSION=$(shell git describe --tags 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +%Y-%m-%d)
LDFLAGS=-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# Binary name
BINARY=surge

build:
	$(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/surge

build/linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/surge

build/darwin:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/surge
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/surge

build/windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/surge

test:
	$(GOTEST) -race -cover -v ./...

test/short:
	$(GOTEST) -short -v ./...

lint:
	golangci-lint run ./...

fmt:
	$(GOFMT) ./...

mod/tidy:
	$(GOMOD) tidy
	$(GOMOD) download

install: build
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)

dev-release: build
	goreleaser --snapshot --clean

dist:
	goreleaser release --clean

clean:
	rm -rf dist/
	rm -f $(BINARY)
	rm -f surge.test

# Cross-platform build
build/all: build/linux build/darwin build/windows

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary for the current platform"
	@echo "  build/all   - Build for all platforms (linux, darwin, windows)"
	@echo "  test        - Run tests with race detection and coverage"
	@echo "  lint        - Run golangci-lint"
	@echo "  fmt         - Format code"
	@echo "  install     - Build and install to /usr/local/bin"
	@echo "  dev-release - Build snapshot release with goreleaser"
	@echo "  dist        - Create official release"
	@echo "  clean       - Remove build artifacts"
