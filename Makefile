.PHONY: setup start build lint test check dev generate

setup:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.11.4

build:
	go build -o bin/aircover cmd/aircover/main.go

start:
	go run cmd/aircover/main.go server

dev:
	go run github.com/air-verse/air@latest -c .air.toml

generate:
	@mkdir -p docs
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0 -package api -generate chi-server,types,spec api/openapi.yaml > internal/api/api.gen.go
	go run cmd/aircover/main.go doc > docs/routes.json

lint:
	@if [ "$$(uname -m)" != "arm64" ]; then \
		echo "Installing golangci-lint"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin latest; \
	fi
	@go mod tidy
	@go vet ./...
	@go fmt ./...
	@output="$$(./bin/golangci-lint run ./... 2>&1)" || { echo "$$output" ; exit 1 ; }
# 	@uvx --from skills-ref agentskills validate ./.agents/skills/verify-changes

test:
	@go test -coverprofile=coverage.out ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	if [ "$$coverage" != "100.0" ]; then \
		echo "Test coverage is $$coverage%, expected 100.0%"; \
		exit 1; \
	fi

check: lint test build
