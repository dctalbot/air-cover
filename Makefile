.PHONY: setup start lint test check

setup:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.11.4

start:
	go run cmd/aircover/main.go

lint:
	@if [ "$$(uname -m)" != "arm64" ]; then \
		echo "Installing golangci-lint"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin latest; \
	fi
	@go mod tidy
	@go vet ./...
	@go fmt ./...
	@output="$$(./bin/golangci-lint run ./... 2>&1)" || { echo "$$output" ; exit 1 ; }

test:
	@go test -coverprofile=coverage.out ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	if [ "$$coverage" != "100.0" ]; then \
		echo "Test coverage is $$coverage%, expected 100.0%"; \
		exit 1; \
	fi

check: lint test
