# Internal Architecture

`internal` follows a direction-explicit hexagonal architecture:

- `domain` contains core entities and rules. It must stay free of app, adapter,
  transport, persistence, config, and logging concerns.
- `app` contains use cases, app read models, and app-owned ports. Ports live near
  the use case that needs them, usually in `ports.go`.
- `adapters/inbound` contains drivers that receive external input and invoke app
  use cases, such as HTTP handlers.
- `adapters/outbound` contains driven implementations of app-owned ports, such as
  repositories, email senders, and third-party API clients.
- `platform` contains process-level technical helpers such as config and logging.
- `cmd` is the composition root. It is the place where concrete adapters,
  app services, routing, config, logging, and process lifecycle are assembled.

When adding a feature, start in the app use case and domain behavior, define the
smallest app-owned port needed by that use case, implement the port in an adapter,
then wire the concrete adapter from `cmd`.

Architectural boundaries are enforced by `internal/architecture_test.go`; update
those tests with any intentional new boundary.
