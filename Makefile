.PHONY: check vet test lint build-release

check: vet test lint

vet:
	go vet ./...

test:
	go test ./...

GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo "$(shell go env GOPATH)/bin/golangci-lint")

lint:
	$(GOLANGCI_LINT) run ./...

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
