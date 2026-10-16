SHELL := sh
.DEFAULT_GOAL := help

APP := ocd-smoke-alarm
GO ?= go

# Pinned so local formatting and CI check exactly the same rules.
# Bump deliberately via PR (see issue #17).
GOFUMPT_VERSION := v0.12.0
GOFUMPT := $(GO) run mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

PKG_ALL := ./...
PKG_UNIT := ./...
PKG_INTEGRATION := ./tests/integration/...

.PHONY: help tidy fmt fmt-check lint test unit integration footprint ci check

help:
	@echo "Available targets:"
	@echo "  make tidy         - Sync module dependencies"
	@echo "  make fmt          - Format Go code (gofumpt, pinned)"
	@echo "  make fmt-check    - Fail if any file is not gofumpt-formatted"
	@echo "  make lint         - Run static analysis (golangci-lint if available)"
	@echo "  make test         - Run all tests (unit + integration packages)"
	@echo "  make unit         - Run unit tests"
	@echo "  make integration  - Run integration tests"
	@echo "  make footprint    - Run repeatable resource footprint checks"
	@echo "  make ci           - Run fmt, lint, unit, integration, footprint"
	@echo "  make check        - Alias for ci"

tidy:
	$(GO) mod tidy

fmt:
	$(GOFUMPT) -w .

fmt-check:
	@unformatted="$$($(GOFUMPT) -l .)" || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofumpt-formatted:"; \
		echo "$$unformatted"; \
		echo "Run 'make fmt' (gofumpt $(GOFUMPT_VERSION)) to fix."; \
		exit 1; \
	fi

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet instead"; \
		$(GO) vet $(PKG_ALL); \
	fi

unit:
	$(GO) test $(PKG_UNIT) -count=1

integration:
	$(GO) test $(PKG_INTEGRATION) -count=1 -v

test: unit integration

footprint:
	@echo "Running resource footprint checks (deterministic settings)..."
	@GOMAXPROCS=1 $(GO) test $(PKG_INTEGRATION) -run 'TestResourceBudget|TestHealthServer_LivenessReadinessAndStatus' -count=1 -v

ci: fmt-check lint test footprint

check: ci
