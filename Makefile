SHELL := sh
.DEFAULT_GOAL := help

APP := ocd-smoke-alarm
GO ?= go

PKG_ALL := ./...
PKG_UNIT := ./...
PKG_INTEGRATION := ./tests/integration/...

.PHONY: help tidy fmt lint test unit integration footprint ci check

help:
	@echo "Available targets:"
	@echo "  make tidy         - Sync module dependencies"
	@echo "  make fmt          - Format Go code"
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
	$(GO) fmt $(PKG_ALL)

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

ci: fmt lint test footprint

check: ci
