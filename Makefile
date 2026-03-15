# Future Render — Build Targets
#
# Usage:
#   make          — run the default CI pipeline (fmt, vet, lint, test, cover-check, build)
#   make ci       — same as default, explicit name for CI systems
#   make test     — run tests
#   make lint     — run golangci-lint
#   make cover    — run tests with coverage summary
#   make cover-check — enforce minimum coverage per package (fails CI if below 80%)
#   make cover-html  — generate HTML coverage report
#   make fix      — auto-fix lint and formatting issues
#   make bench    — run benchmarks
#   make clean    — remove build artifacts
#
# Prerequisites:
#   go 1.24+
#   golangci-lint (https://golangci-lint.run/welcome/install/)

.PHONY: all ci fmt vet lint test test-race bench build clean fix check-lint cover cover-check cover-html

# Minimum required test coverage per package (percentage).
COVERAGE_MIN := 80

# Packages to build/test. Audio packages require platform-specific C libraries
# (ALSA on Linux) that may not be available in all environments, so they are
# excluded from the default package set. Use "go test ./audio/..." directly
# when ALSA development headers are installed.
PKGS := $(shell go list ./... | grep -v /audio | grep -v cmd/audio)

# LINT_PATHS provides relative directory paths for golangci-lint, which
# requires filesystem paths rather than Go module paths.
MODULE := $(shell go list -m)
LINT_PATHS := $(shell go list ./... | grep -v /audio | grep -v cmd/audio | sed "s|^$(MODULE)|.|")

# Default target runs the full CI pipeline
all: ci

# --- CI Pipeline (order matters) ---

ci: fmt vet lint test cover-check build
	@echo "CI pipeline passed."

# --- Individual Targets ---

# Check formatting (fails if files need formatting)
fmt:
	@echo "==> Checking formatting..."
	@test -z "$$(gofmt -l .)" || { echo "Files need formatting:"; gofmt -l .; exit 1; }

# Go vet
vet:
	@echo "==> Running go vet..."
	go vet $(PKGS)

# Lint with golangci-lint
lint: check-lint
	@echo "==> Running golangci-lint..."
	golangci-lint run $(LINT_PATHS)

# Run all tests
test:
	@echo "==> Running tests..."
	go test $(PKGS)

# Run tests with race detector
test-race:
	@echo "==> Running tests with race detector..."
	go test -race $(PKGS)

# Run benchmarks
bench:
	@echo "==> Running benchmarks..."
	go test -bench=. -benchmem ./math/ ./internal/batch/

# Build all packages
build:
	@echo "==> Building..."
	go build $(PKGS)

# --- Coverage Targets ---

# Run tests and print per-package coverage summary
cover:
	@echo "==> Running tests with coverage..."
	@go test -cover $(PKGS)

# Enforce minimum coverage per package.
# - Lines starting with "ok" have tests — enforce COVERAGE_MIN%.
# - Lines without "ok" are dependency-only (no test files) — warn unless excluded.
# - Interface-only packages (backend, platform) are excluded.
cover-check:
	@echo "==> Checking coverage (minimum $(COVERAGE_MIN)%)..."
	@go test -cover $(PKGS) 2>&1 | awk -v min=$(COVERAGE_MIN) ' \
	/^ok/ && /coverage:/ { \
		pkg = $$2; \
		for (i = 1; i <= NF; i++) { \
			if ($$i == "coverage:") { \
				pct = $$(i+1); \
				gsub(/%/, "", pct); \
				break; \
			} \
		} \
		if (pct + 0 < min) { \
			fail[pkg] = pct; \
		} else { \
			pass[pkg] = pct; \
		} \
		next; \
	} \
	/coverage: 0.0%/ && !/^ok/ { \
		pkg = $$1; \
		if (pkg !~ /\/backend$$/ && pkg !~ /\/platform$$/) { \
			warn[pkg] = 1; \
		} \
		next; \
	} \
	END { \
		for (p in pass) printf "  ✓ %-55s %5.1f%%\n", p, pass[p]; \
		for (p in fail) printf "  ✗ %-55s %5.1f%% (minimum: %d%%)\n", p, fail[p], min; \
		for (w in warn) printf "  ⚠ %-55s no test files\n", w; \
		if (length(fail) > 0) { \
			printf "\nFAIL: %d package(s) below %d%% coverage.\n", length(fail), min; \
			exit 1; \
		} \
		if (length(warn) > 0) { \
			printf "\nWARN: %d package(s) have no test files.\n", length(warn); \
		} \
		printf "Coverage check passed.\n"; \
	}'

# Generate HTML coverage report
cover-html:
	@echo "==> Generating coverage report..."
	@go test -coverprofile=cover.out $(PKGS)
	@go tool cover -html=cover.out -o coverage.html
	@echo "Coverage report: coverage.html"

# --- Fix & Clean ---

# Auto-fix formatting and lint issues
fix: check-lint
	@echo "==> Fixing formatting..."
	gofmt -w .
	@echo "==> Fixing lint issues..."
	golangci-lint run --fix $(LINT_PATHS)

# Remove build artifacts
clean:
	@echo "==> Cleaning..."
	go clean $(PKGS)
	rm -f cover.out coverage.html

# --- Tool Checks ---

check-lint:
	@which golangci-lint > /dev/null 2>&1 || { \
		echo "golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	}
