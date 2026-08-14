.PHONY: check vet test lint build-release

# Recipes must run from the module root. `make -f ../Makefile lint` from
# dist/ or .github/ would otherwise pass ./... to a directory with no Go
# files, and golangci-lint 2.12.2 reports that as "no go files to analyze"
# — a loader/cwd error, not a clean lint result.
MODULE_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

check: vet test lint

vet:
	cd "$(MODULE_ROOT)" && go vet ./...

test:
	cd "$(MODULE_ROOT)" && go test ./...

GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo "$(shell go env GOPATH)/bin/golangci-lint")

lint:
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found at $(GOLANGCI_LINT)"; \
		echo "install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2"; \
		exit 1; \
	fi
	cd "$(MODULE_ROOT)" && $(GOLANGCI_LINT) run ./...

# Local multi-arch build (mirrors release.yml). VERSION defaults to git describe.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
build-release:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/gsc-mcp-darwin-arm64 ./cmd/gsc-mcp
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/gsc-mcp-darwin-amd64 ./cmd/gsc-mcp
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/gsc-mcp-linux-amd64 ./cmd/gsc-mcp
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/gsc-mcp-linux-arm64 ./cmd/gsc-mcp
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/gsc-mcp-windows-amd64.exe ./cmd/gsc-mcp
	(cd dist && shasum -a 256 gsc-mcp-* > checksums.txt)
	@echo "VERSION=$(VERSION)"
	@cat dist/checksums.txt
