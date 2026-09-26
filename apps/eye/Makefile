# Makefile — eye
# One binary, no mandatory services. Everything below runs offline.

BINARY   := eye
OUTPUT   := bin/$(BINARY)
CMD_PATH := ./cmd/eye

VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG      := github.com/FullFran/cordvba/apps/eye/internal/version
LDFLAGS  := -s -w \
            -X $(PKG).Version=$(VERSION) \
            -X $(PKG).Commit=$(COMMIT) \
            -X $(PKG).Date=$(DATE)

GO_FLAGS := CGO_ENABLED=0
GOLANGCI := golangci-lint

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	     /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: run
run: ## Run eye from source (make run ARGS="status")
	go run $(CMD_PATH) $(ARGS)

.PHONY: build
build: ## Compile a static, CGO-free binary to $(OUTPUT)
	@mkdir -p bin
	$(GO_FLAGS) go build -trimpath -ldflags="$(LDFLAGS)" -o $(OUTPUT) $(CMD_PATH)

.PHONY: install
install: ## Install eye into $(GOPATH)/bin
	$(GO_FLAGS) go install -trimpath -ldflags="$(LDFLAGS)" $(CMD_PATH)

.PHONY: test
test: ## Run all tests with race detector and coverage
	go test -race -cover ./...

.PHONY: cover
cover: ## Write and open an HTML coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI) run ./...

.PHONY: fmt
fmt: ## Format code with gofmt
	gofmt -l -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: ci-local
ci-local: fmt vet lint test build ## Run the full CI pipeline locally
	@echo "CI pipeline passed."

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out
