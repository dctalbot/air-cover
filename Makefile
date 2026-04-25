.PHONY: setup start lint

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
