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

.PHONY: all ci fmt vet lint test test-race bench build build-android clean fix check-lint cover cover-check cover-html

# Minimum required test coverage per package (percentage).
COVERAGE_MIN := 80

# Build tags. Set TAGS=soft for CI (no GPU hardware).
# GPU bindings compile by default; -tags soft switches to soft-delegation.
TAGS ?=
ifneq ($(TAGS),)
  GO_TAGS := -tags "$(TAGS)"
endif

# Packages for vet/lint/test/coverage. Excludes:
# - audio/: requires ALSA headers (CGo) on Linux
# - cmd/: example binaries with no test files
# - internal/gl, internal/platform/glfw, internal/platform/cocoa,
#   internal/backend/opengl: purego interop requires uintptr→unsafe.Pointer
#   conversions that go vet flags; no tests in CI
# - internal/mtl, internal/vk, internal/wgpu, internal/d3d12: GPU binding packages
#   use purego/unsafe; excluded from vet/lint (tested via backend conformance)
PKGS := $(shell go list -e $(GO_TAGS) ./... 2>/dev/null | grep -v /audio | grep -v /cmd/ | grep -v /internal/gl$$ | grep -v /internal/platform/glfw | grep -v /internal/platform/cocoa | grep -v /internal/platform/android | grep -v /internal/backend/opengl | grep -v /internal/mtl$$ | grep -v /internal/vk$$ | grep -v /internal/wgpu$$ | grep -v /internal/d3d12$$)

# LINT_PATHS provides relative directory paths for golangci-lint, which
# requires filesystem paths rather than Go module paths.
MODULE := $(shell go list -m)
LINT_PATHS := $(shell go list -e $(GO_TAGS) ./... 2>/dev/null | grep -v /audio | grep -v /cmd/ | grep -v /internal/gl$$ | grep -v /internal/platform/glfw | grep -v /internal/platform/cocoa | grep -v /internal/platform/android | grep -v /internal/backend/opengl | grep -v /internal/mtl$$ | grep -v /internal/vk$$ | grep -v /internal/wgpu$$ | grep -v /internal/d3d12$$ | sed "s|^$(MODULE)|.|")

# All buildable packages. Excludes:
# - audio/: requires ALSA headers (CGo) on Linux
BUILD_PKGS := $(shell go list -e $(GO_TAGS) ./... 2>/dev/null | grep -v /audio)

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
# -unsafeptr=false: purego-based platform packages (cocoa, win32) use
# uintptr→unsafe.Pointer casts that are valid for the ObjC/Win32 runtime
# but flagged by go vet's unsafeptr analyzer.
vet:
	@echo "==> Running go vet..."
	go vet -unsafeptr=false $(GO_TAGS) $(PKGS)

# Lint with golangci-lint
lint: check-lint
	@echo "==> Running golangci-lint..."
	golangci-lint run $(if $(TAGS),--build-tags "$(TAGS)") $(LINT_PATHS)

# Run all tests
test:
	@echo "==> Running tests..."
	go test $(GO_TAGS) $(PKGS)

# Run tests with race detector
test-race:
	@echo "==> Running tests with race detector..."
	go test -race $(GO_TAGS) $(PKGS)

# Run benchmarks
bench:
	@echo "==> Running benchmarks..."
	go test -bench=. -benchmem $(GO_TAGS) ./math/ ./internal/batch/

# Build all packages (includes cmd/ examples and platform code)
build:
	@echo "==> Building..."
	go build $(GO_TAGS) $(BUILD_PKGS)

# Cross-compile for Android (arm64). Verifies engine-internal packages compile
# for Android without requiring the NDK or CGo. The root package, platform
# packages (glfw, cocoa, win32), and GPU binding packages are excluded because
# they are desktop-only. The root package imports x/mobile/app which requires
# JNI bindings via CGo. Full Android builds need CGO_ENABLED=1 with the NDK.
build-android:
	@echo "==> Building for Android (arm64)..."
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -tags soft \
		$(shell GOOS=android GOARCH=arm64 go list -tags soft ./... 2>/dev/null \
		| grep -v "github.com/michaelraines/future-core$$" \
		| grep -v /audio \
		| grep -v /cmd/ \
		| grep -v /text$$ \
		| grep -v /internal/platform/glfw \
		| grep -v /internal/platform/cocoa \
		| grep -v /internal/platform/win32 \
		| grep -v /internal/platform/android \
		| grep -v /internal/gl$$ \
		| grep -v /internal/mtl$$ \
		| grep -v /internal/vk$$ \
		| grep -v /internal/wgpu$$ \
		| grep -v /internal/d3d12$$ \
		| grep -v /internal/backend/opengl)

# --- Coverage Targets ---

# Run tests and print per-package coverage summary
cover:
	@echo "==> Running tests with coverage..."
	@go test -cover $(GO_TAGS) $(PKGS)

# Enforce minimum coverage per package.
# - Lines starting with "ok" have tests — enforce COVERAGE_MIN%.
# - Lines without "ok" are dependency-only (no test files) — warn unless excluded.
# - Interface-only packages (backend, platform) are excluded.
cover-check:
	@echo "==> Checking coverage (minimum $(COVERAGE_MIN)%)..."
	@go test -cover $(GO_TAGS) $(PKGS) 2>&1 | awk -v min=$(COVERAGE_MIN) ' \
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
	@go test -coverprofile=cover.out $(GO_TAGS) $(PKGS)
	@go tool cover -html=cover.out -o coverage.html
	@echo "Coverage report: coverage.html"

# --- Fix & Clean ---

# Auto-fix formatting and lint issues
fix: check-lint
	@echo "==> Fixing formatting..."
	gofmt -w .
	@echo "==> Fixing lint issues..."
	golangci-lint run --fix $(if $(TAGS),--build-tags "$(TAGS)") $(LINT_PATHS)

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
