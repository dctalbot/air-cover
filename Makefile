.PHONY: setup start build lint test check generate codecov-html coverage-summary db-reset db-down db-status

setup:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.11.4

build:
	go build -o bin/aircover cmd/aircover/main.go

start:
	@lsof -ti :8080 | xargs -r kill -TERM 2>/dev/null || true
	go run github.com/air-verse/air@latest -c .air.toml

generate:
	@mkdir -p docs
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0 -package api -generate chi-server,types,spec api/openapi.yaml > internal/api/api.gen.go
	@go run cmd/aircover/main.go doc > docs/routes.json
	@go run github.com/a-h/templ/cmd/templ@latest generate

db-reset:
	go run cmd/aircover/main.go migrate reset

db-down:
	go run cmd/aircover/main.go migrate down

db-status:
	go run cmd/aircover/main.go migrate status

lint:
	@if [ "$$(uname -m)" != "arm64" ]; then \
		echo "Installing golangci-lint"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin latest; \
	fi
	@./bin/golangci-lint config verify
	@go mod tidy
	@go vet ./...
	@go fmt ./...
	@output="$$(./bin/golangci-lint run ./... 2>&1)" || { echo "$$output" ; exit 1 ; }
# 	@uvx --from skills-ref agentskills validate ./.agents/skills/verify-changes

test:
	@go test -coverprofile=coverage.out ./...
	@grep -v '\.gen\.go' coverage.out | grep -v '_templ\.go' > coverage.filtered.out
	@coverage=$$(go tool cover -func=coverage.filtered.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	if [ "$$coverage" != "100.0" ]; then \
		echo "Test coverage is $$coverage%, expected 100.0%"; \
		exit 1; \
	fi

check: lint test build

coverage-summary: test
	@echo "Raw coverage:"
	@go tool cover -func=coverage.out | grep total:
	@echo "Filtered coverage (excluding generated OpenAPI and templ output):"
	@go tool cover -func=coverage.filtered.out | grep total:

codecov-html:
	go tool cover -html=coverage.out
