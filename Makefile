GO_VERSION := 1.26.3
GO := GOTOOLCHAIN=go$(GO_VERSION) go
include tools.mk

.PHONY: setup start build lint test vuln check generate codecov-html coverage-summary db-reset db-down db-status

setup:
	GOBIN="$$(pwd)/bin" $(GO) install $(GOLANGCI_LINT)
	GOBIN="$$(pwd)/bin" $(GO) install $(GOVULNCHECK)

build:
	$(GO) build -o bin/aircover cmd/aircover/main.go

start:
	@lsof -ti :8080 | xargs -r kill -TERM 2>/dev/null || true
	$(GO) run $(AIR) -c .air.toml

generate:
	@mkdir -p docs
	@$(GO) run $(SQLC) generate
	@$(GO) run $(OAPI_CODEGEN) -package httpadapter -generate chi-server,types,spec api/openapi.yaml > internal/adapters/http/api.gen.go
	@$(GO) run cmd/aircover/main.go doc > docs/routes.json
	@$(GO) run $(TEMPL) generate

db-reset:
	$(GO) run cmd/aircover/main.go migrate reset

db-down:
	$(GO) run cmd/aircover/main.go migrate down

db-status:
	$(GO) run cmd/aircover/main.go migrate status

lint:
	@GOBIN="$$(pwd)/bin" $(GO) install $(GOLANGCI_LINT)
	@./bin/golangci-lint config verify
	@$(GO) mod tidy
	@$(GO) vet ./...
	@$(GO) fmt ./...
	@output="$$(./bin/golangci-lint run ./... 2>&1)" || { echo "$$output" ; exit 1 ; }
# 	@uvx --from skills-ref agentskills validate ./.agents/skills/verify-changes

test:
	@$(GO) test -count=1 -coverprofile=coverage.out ./...
	@{ IFS= read -r mode; printf '%s\n' "$$mode"; LC_ALL=C sort; } < coverage.out > coverage.sorted.out
	@mv coverage.sorted.out coverage.out
	@grep -v '\.gen\.go' coverage.out | grep -v '_templ\.go' > coverage.filtered.out
	@{ IFS= read -r mode; printf '%s\n' "$$mode"; LC_ALL=C sort; } < coverage.filtered.out > coverage.filtered.sorted.out
	@mv coverage.filtered.sorted.out coverage.filtered.out
	@coverage=$$($(GO) tool cover -func=coverage.filtered.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	if [ "$$coverage" != "100.0" ]; then \
		echo "Test coverage is $$coverage%, expected 100.0%"; \
		exit 1; \
	fi

vuln:
	@GOBIN="$$(pwd)/bin" $(GO) install $(GOVULNCHECK)
	@output="$$(./bin/govulncheck ./... 2>&1)"; status=$$?; \
	if [ $$status -ne 0 ] || ! printf '%s\n' "$$output" | grep -q 'No vulnerabilities found.'; then \
		printf '%s\n' "$$output"; \
	fi; \
	exit $$status

check: lint test vuln build

coverage-summary: test
	@echo "Raw coverage:"
	@$(GO) tool cover -func=coverage.out | grep total:
	@echo "Filtered coverage (excluding generated OpenAPI and templ output):"
	@$(GO) tool cover -func=coverage.filtered.out | grep total:

codecov-html:
	$(GO) tool cover -html=coverage.out
