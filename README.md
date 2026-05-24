# Air Cover

Air Cover is a Spinitron integration that allows DJs (Personas) to request substitutes to cover their shows, and in turn, allows them to pick up other substitution requests.

## Development setup

```
make setup
```

## Architecture

This project is built using Go. The general structure follows standard Go project layouts:

- `cmd/aircover/`: Contains the main application entry point.
- `internal/`: Contains private application code.
  - `internal/domain/`: Domain entities and behavior (users, auth, substitution requests, etc.)
  - `internal/spinitron/`: Client for interacting with the Spinitron API.
  - `internal/api/`: Handlers for the application's HTTP API.

## Getting Started

1. Ensure you have Go 1.26.0 or later installed.
2. Clone the repository.
3. Run the application:
   ```sh
   go run cmd/aircover/main.go
   ```

Set required environment variables before running:
- `DB_URI`
- `SPINITRON_API_URL`

## Spinitron Integration

(WIP) Documentation on how to configure Spinitron API keys and webhooks.
