.PHONY: help test test-race test-coverage lint fmt vet clean install-tools benchmarks docs build all \
	examples run-examples run-example docs-server readme-generate api-docs check-example lint-examples

# Variables
BINARY_NAME=niyantrak
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html
MIN_COVERAGE=90
GO_VERSION:=$(shell go version | awk '{print $$3}')
EXAMPLES_DIR=examples
DOCS_DIR=docs

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

##@ Help
help: ## Display this help screen
	@echo "$(GREEN)Niyantrak Rate Limiter Library - Makefile$(NC)"
	@echo "$(YELLOW)Available targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make test           # Run all tests"
	@echo "  make test-race      # Run tests with race detector"
	@echo "  make lint           # Run all linters"
	@echo "  make coverage       # Generate coverage report"

##@ Examples & Learning
examples: ## List all available examples
	@echo "$(GREEN)Available Examples:$(NC)"
	@echo ""
	@echo "$(YELLOW)Core Rate Limiting Examples:$(NC)"
	@for dir in $(EXAMPLES_DIR)/0*; do \
		name=$$(basename $$dir); \
		echo "  $(GREEN)$$name$(NC)"; \
		head -3 $$dir/main.go | grep "^//" | sed 's|^//[ ]*||'; \
	done
	@echo ""
	@echo "$(YELLOW)Usage:$(NC)"
	@echo "  make run-example NUM=01    # Run example 01_basic_memory"
	@echo "  make run-examples          # Run all examples"
	@echo "  make docs-examples         # View example documentation"

run-example: ## Run a specific example (use NUM=01 for example 01_basic_memory)
	@if [ -z "$(NUM)" ]; then \
		echo "$(RED)Error: NUM not specified$(NC)"; \
		echo "Usage: make run-example NUM=01"; \
		exit 1; \
	fi
	@EXAMPLE_DIR="$(EXAMPLES_DIR)/$$(printf '%02d' $(NUM))*"; \
	if [ ! -d "$$EXAMPLE_DIR" ]; then \
		echo "$(RED)Error: Example $(NUM) not found$(NC)"; \
		ls -d $(EXAMPLES_DIR)/* 2>/dev/null | grep -o '[0-9]*_[^/]*' | sort; \
		exit 1; \
	fi; \
	echo "$(GREEN)Running $$EXAMPLE_DIR...$(NC)"; \
	go run $$EXAMPLE_DIR/main.go

run-examples: ## Run all examples sequentially
	@echo "$(GREEN)Running all examples...$(NC)"
	@for dir in $(EXAMPLES_DIR)/0*; do \
		name=$$(basename $$dir); \
		echo ""; \
		echo "$(YELLOW)▶ Running $$name$(NC)"; \
		go run $$dir/main.go; \
		echo "$(GREEN)✓ $$name completed$(NC)"; \
	done
	@echo ""
	@echo "$(GREEN)✓ All examples completed$(NC)"

lint-examples: ## Lint all example files
	@echo "$(GREEN)Linting example files...$(NC)"
	@for dir in $(EXAMPLES_DIR)/0*; do \
		if [ -f "$$dir/main.go" ]; then \
			echo "  Checking $$dir/main.go..."; \
			gofmt -l "$$dir/main.go"; \
			go vet "$$dir"; \
		fi \
	done
	@echo "$(GREEN)✓ Example linting completed$(NC)"

docs-examples: ## View examples documentation
	@echo "$(GREEN)Examples Documentation$(NC)"
	@cat $(EXAMPLES_DIR)/README.md

example-config: ## Display example configuration reference
	@echo "$(GREEN)Configuration Reference$(NC)"
	@cat $(EXAMPLES_DIR)/config.yaml

test-examples: ## Build/check all examples compile correctly
	@echo "$(GREEN)Testing all examples compile...$(NC)"
	@for dir in $(EXAMPLES_DIR)/0*; do \
		if [ -f "$$dir/main.go" ]; then \
			echo "  Building $$dir..."; \
			go build -o /tmp/$$(basename $$dir) $$dir/main.go || exit 1; \
		fi \
	done
	@rm -f /tmp/01_* /tmp/02_* /tmp/03_* /tmp/04_* /tmp/05_* /tmp/06_*
	@echo "$(GREEN)✓ All examples compile successfully$(NC)"

test: ## Run all tests
	@echo "$(GREEN)Running tests...$(NC)"
	@go test -v -timeout 5m ./...
	@echo "$(GREEN)✓ Tests passed$(NC)"

test-race: ## Run tests with race condition detector (slower, finds data races)
	@echo "$(GREEN)Running tests with race detector...$(NC)"
	@go test -v -race -timeout 10m ./...
	@echo "$(GREEN)✓ Tests passed (no data races detected)$(NC)"

test-short: ## Run tests in short mode (faster, good for development)
	@echo "$(GREEN)Running tests (short mode)...$(NC)"
	@go test -v -short -timeout 2m ./...
	@echo "$(GREEN)✓ Tests passed$(NC)"

coverage: test-coverage ## Alias for test-coverage

test-coverage: ## Run tests and generate coverage report
	@echo "$(GREEN)Running tests with coverage analysis...$(NC)"
	@go test -v -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(GREEN)✓ Coverage report generated: $(COVERAGE_HTML)$(NC)"
	@go tool cover -func=$(COVERAGE_FILE) | tail -1

coverage-report: test-coverage ## Display coverage report in terminal
	@go tool cover -func=$(COVERAGE_FILE)

##@ Code Quality
lint: fmt vet staticcheck golangci-lint ## Run all linters (fmt, vet, staticcheck, golangci-lint)

fmt: ## Check code formatting with gofmt
	@echo "$(GREEN)Checking code formatting...$(NC)"
	@gofmt -l -w .
	@echo "$(GREEN)✓ Code formatted$(NC)"

vet: ## Run go vet for suspicious constructs
	@echo "$(GREEN)Running go vet...$(NC)"
	@go vet -v ./...
	@echo "$(GREEN)✓ Go vet passed$(NC)"

staticcheck: install-tools ## Run staticcheck for code analysis
	@echo "$(GREEN)Running staticcheck...$(NC)"
	@staticcheck -checks=all ./...
	@echo "$(GREEN)✓ Staticcheck passed$(NC)"

golangci-lint: install-tools ## Run golangci-lint
	@echo "$(GREEN)Running golangci-lint...$(NC)"
	@golangci-lint run --timeout=5m ./...
	@echo "$(GREEN)✓ Golangci-lint passed$(NC)"

gosec: install-tools ## Run gosec for security scanning
	@echo "$(GREEN)Running gosec (security scanner)...$(NC)"
	@gosec -no-fail -fmt sarif -out gosec-report.sarif ./...
	@echo "$(GREEN)✓ Gosec scan completed (report: gosec-report.sarif)$(NC)"

tidy: ## Tidy up go.mod and verify dependencies
	@echo "$(GREEN)Tidying dependencies...$(NC)"
	@go mod tidy
	@go mod verify
	@echo "$(GREEN)✓ Dependencies tidied and verified$(NC)"

##@ Benchmarking
benchmarks: install-tools ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	@go test -v -run=^$$ -bench=. -benchmem -benchtime=10s ./...
	@echo "$(GREEN)✓ Benchmarks completed$(NC)"

bench-memory: ## Run memory benchmarks specifically
	@echo "$(GREEN)Running memory benchmarks...$(NC)"
	@go test -v -run=^$$ -bench=BenchmarkMemory -benchmem -benchtime=10s ./benchmarks

bench-compare: benchmarks ## Run and compare benchmarks (requires benchstat)
	@echo "$(GREEN)Benchmarks require benchstat for comparison$(NC)"
	@go install golang.org/x/perf/cmd/benchstat@latest

##@ Build & Development
build: ## Build the library (generates no binary, just validates)
	@echo "$(GREEN)Building library...$(NC)"
	@go build -v ./...
	@echo "$(GREEN)✓ Build successful$(NC)"

clean: ## Remove generated files and artifacts
	@echo "$(GREEN)Cleaning up...$(NC)"
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@rm -f gosec-report.sarif
	@go clean -testcache
	@rm -rf dist/ vendor/
	@echo "$(GREEN)✓ Cleanup completed$(NC)"

install-tools: ## Install required development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	@command -v gosec >/dev/null 2>&1 || go install github.com/securego/gosec/v2/cmd/gosec@latest
	@echo "$(GREEN)✓ Tools installed$(NC)"

##@ Documentation
docs: docs-server ## Serve documentation locally

docs-server: ## Start local documentation server (http://localhost:6060)
	@echo "$(GREEN)Starting documentation server...$(NC)"
	@echo "$(YELLOW)Open: http://localhost:6060/pkg/github.com/abhipray-cpu/niyantrak$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	@godoc -http=:6060

docs-build: ## Build documentation (outputs to stdout)
	@echo "$(GREEN)Building documentation...$(NC)"
	@go doc -all ./...

readme-generate: ## Generate/update API documentation from code comments
	@echo "$(GREEN)Extracting API documentation...$(NC)"
	@echo "# API Reference" > docs/api-reference.md
	@echo "" >> docs/api-reference.md
	@echo "Generated from code comments. Run \`go doc -all\` for detailed documentation." >> docs/api-reference.md
	@echo "" >> docs/api-reference.md
	@go doc -all ./... >> docs/api-reference.md 2>&1 || true
	@echo "$(GREEN)✓ API reference updated: docs/api-reference.md$(NC)"

api-docs: ## Display API documentation for specific package (use PKG=algorithm)
	@if [ -z "$(PKG)" ]; then \
		echo "$(RED)Error: PKG not specified$(NC)"; \
		echo "Usage: make api-docs PKG=algorithm"; \
		echo ""; \
		echo "Available packages:"; \
		ls -d */ | grep -v '^\.' | sed 's|/$||' | sort | sed 's/^/  /'; \
		exit 1; \
	fi
	@echo "$(GREEN)Documentation for $(PKG):$(NC)"
	@go doc ./$(PKG)

docs-watch: ## Watch for changes and rebuild docs
	@echo "$(GREEN)Watching documentation...$(NC)"
	@find $(DOCS_DIR) -name '*.md' | entr make docs-build

arch-diagram: ## Display architecture diagram
	@cat docs/architecture-diagram.md

##@ Integration & Quality Assurance
all: clean tidy fmt vet lint test test-race coverage ## Run full quality assurance (clean, tidy, fmt, vet, lint, test, race, coverage)
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║  All quality checks passed! ✓         ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════╝$(NC)"
	@echo ""

ci-check: tidy lint test test-race coverage gosec ## Run CI checks (tidy, lint, test, race, coverage, security)
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║  CI checks passed! ✓                   ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════╝$(NC)"
	@echo ""

##@ Project Information
info: ## Display project information
	@echo "$(GREEN)Project: Niyantrak - Go Rate Limiter Library$(NC)"
	@echo "$(GREEN)Go Version: $(GO_VERSION)$(NC)"
	@echo "$(GREEN)Module: github.com/abhipray-cpu/niyantrak$(NC)"
	@echo ""
	@echo "$(YELLOW)Directory Structure:$(NC)"
	@echo "  algorithm/     - Rate limiting algorithms"
	@echo "  backend/       - Storage backends (Memory, Redis, PostgreSQL)"
	@echo "  middleware/    - HTTP and gRPC middleware"
	@echo "  metrics/       - Observability and metrics"
	@echo "  config/        - Configuration structures"
	@echo "  examples/      - Example implementations"
	@echo "  internal/      - Private packages"
	@echo "  docs/          - Documentation"
	@echo "  benchmarks/    - Performance benchmarks"

version: ## Display version information
	@echo "Niyantrak v1.0.0-dev"
	@echo "Go $(GO_VERSION)"

##@ Utilities
check-env: ## Check development environment setup
	@echo "$(GREEN)Checking development environment...$(NC)"
	@command -v go >/dev/null 2>&1 && echo "✓ Go installed: $$(go version)" || echo "✗ Go not found"
	@command -v git >/dev/null 2>&1 && echo "✓ Git installed" || echo "✗ Git not found"
	@command -v golangci-lint >/dev/null 2>&1 && echo "✓ golangci-lint installed" || echo "⚠ golangci-lint not found (run: make install-tools)"
	@command -v staticcheck >/dev/null 2>&1 && echo "✓ staticcheck installed" || echo "⚠ staticcheck not found (run: make install-tools)"
	@command -v entr >/dev/null 2>&1 && echo "✓ entr installed" || echo "⚠ entr not found (for watch targets)"
	@echo ""

quick-check: fmt vet test ## Quick check (fmt, vet, test) - useful during development
	@echo "$(GREEN)Quick check completed!$(NC)"

quick-all: quick-check lint ## Quick full check (fmt, vet, test, lint)
	@echo "$(GREEN)✓ Quick full check completed!$(NC)"

watch: ## Watch for changes and run tests (requires entr)
	@echo "$(GREEN)Watching for changes...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	@find . -name '*.go' | entr make test-short

watch-lint: ## Watch for changes and run linters (requires entr)
	@echo "$(GREEN)Watching for changes...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	@find . -name '*.go' | entr make fmt

format-check: ## Check which files need formatting
	@echo "$(GREEN)Checking formatting...$(NC)"
	@gofmt -l .
	@if [ -z "$$(gofmt -l .)" ]; then \
		echo "$(GREEN)✓ All files properly formatted$(NC)"; \
	fi

deps-list: ## List all dependencies with versions
	@echo "$(GREEN)Dependencies:$(NC)"
	@go list -m all

deps-graph: ## Show dependency graph (requires graphviz)
	@echo "$(GREEN)Generating dependency graph...$(NC)"
	@go mod graph | dot -Tsvg > deps.svg && echo "✓ Dependency graph: deps.svg"

module-info: ## Display module information
	@echo "$(GREEN)Module Information:$(NC)"
	@go mod why -m all

stats: ## Display code statistics
	@echo "$(GREEN)Code Statistics:$(NC)"
	@echo ""
	@echo "$(YELLOW)Lines of Code:$(NC)"
	@find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' | xargs wc -l | tail -1
	@echo ""
	@echo "$(YELLOW)File Count:$(NC)"
	@find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' | wc -l | awk '{print $$1 " Go files"}'
	@echo ""
	@echo "$(YELLOW)Package Count:$(NC)"
	@find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' -exec grep -l '^package' {} \; | sed 's|/.*||' | sort -u | wc -l | awk '{print $$1 " packages"}'

tree: ## Display directory tree structure
	@echo "$(GREEN)Directory Structure:$(NC)"
	@find . -type d -not -path '*/\.*' -not -path '*/vendor/*' | head -20 | sed 's|[^/]*/|  |g'

setup-workspace: install-tools check-env ## Complete workspace setup (install tools + check environment)
	@echo "$(GREEN)╔════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║  Workspace setup completed! ✓         ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(YELLOW)Next steps:$(NC)"
	@echo "  1. make quick-check     # Run quick validation"
	@echo "  2. make examples        # View available examples"
	@echo "  3. make run-examples    # Run all examples"
	@echo "  4. make docs-server     # Start documentation server"

help-detailed: ## Display detailed help with all available targets
	@echo "$(GREEN)Niyantrak - Comprehensive Make Targets$(NC)"
	@echo ""
	@echo "$(YELLOW)Quick Start:$(NC)"
	@echo "  make setup-workspace    # Initialize your workspace"
	@echo "  make quick-check        # Validate code quality"
	@echo "  make run-examples       # Run all examples"
	@echo ""
	@echo "$(YELLOW)Full Target List:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-25s$(NC) %s\n", $$1, $$2}'

##@ Git & Release

check-git: ## Check git status and require clean working directory
	@if ! git diff --quiet; then \
		echo "$(RED)Error: Working directory has uncommitted changes$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Working directory is clean$(NC)"

##@ Debugging
debug-test: ## Run tests with verbose output and stop on first failure
	@echo "$(GREEN)Running tests with debug output...$(NC)"
	@go test -v -failfast ./...

debug-example: ## Debug a specific example (use NUM=01)
	@if [ -z "$(NUM)" ]; then \
		echo "$(RED)Error: NUM not specified$(NC)"; \
		echo "Usage: make debug-example NUM=01"; \
		exit 1; \
	fi
	@EXAMPLE_DIR="$(EXAMPLES_DIR)/$$(printf '%02d' $(NUM))*"; \
	echo "$(GREEN)Debugging $$EXAMPLE_DIR with dlv...$(NC)"; \
	dlv debug $$EXAMPLE_DIR/main.go

profile-cpu: ## Generate CPU profile
	@echo "$(GREEN)Generating CPU profile...$(NC)"
	@go test -cpuprofile=cpu.prof -bench=. ./benchmarks
	@go tool pprof cpu.prof

profile-mem: ## Generate memory profile
	@echo "$(GREEN)Generating memory profile...$(NC)"
	@go test -memprofile=mem.prof -bench=. ./benchmarks
	@go tool pprof mem.prof

profile-trace: ## Generate execution trace
	@echo "$(GREEN)Generating execution trace...$(NC)"
	@go test -trace=trace.out ./...
	@go tool trace trace.out

profile-clean: ## Clean up profile files
	@echo "$(GREEN)Cleaning profile files...$(NC)"
	@rm -f cpu.prof mem.prof trace.out
	@echo "$(GREEN)✓ Profile files cleaned$(NC)"

# Development shortcuts
dev-setup: install-tools check-env test examples ## Complete developer setup
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║  Developer setup complete! ✓          ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════╝$(NC)"

dev-serve: docs-server ## Start development server (documentation)

validate: lint test coverage ## Validate code before commit

pre-commit: fmt tidy lint ## Pre-commit checks (format, tidy, lint)

clean-all: clean profile-clean ## Deep clean (remove all generated files)
	@echo "$(GREEN)✓ Complete cleanup done$(NC)"

##@ Default

.DEFAULT_GOAL := help

# Print recipes, don't execute them
print-%:
	@echo $* = $($*)
