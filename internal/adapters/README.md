# Adapter Organization

Adapters translate between external systems and app-owned ports. They may import
`internal/app` and `internal/domain`, but app and domain packages must not import
adapters.

Adapter paths make direction explicit:

- `inbound` adapters drive the application from the outside.
- `outbound` adapters are driven by the application through app-owned ports.

`internal/adapters/inbound` is the inbound HTTP adapter. Its packages keep
API behavior, view models, and rendering separate:

- `internal/adapters/inbound/api` owns the OpenAPI spec, generated API
  types, HTTP handlers, middleware, and router;
- `internal/adapters/inbound/presenter` owns HTTP view-model formatting;
- `internal/adapters/inbound/ui` owns templ rendering.

The HTTP adapter should:

- parse HTTP requests, cookies, headers, and route params;
- call app-owned use-case interfaces;
- map app errors to HTTP status codes;
- delegate view-model formatting to `internal/adapters/inbound/presenter` and rendering to `internal/adapters/inbound/ui`.

Do not add business rules, repository interfaces, or database/provider details to `internal/adapters/inbound`.

`internal/adapters/outbound` packages implement app-owned ports:

- `internal/adapters/outbound/sqlite` implements repository ports and keeps sqlc-generated types under `internal/adapters/outbound/sqlite/dbgen`;
- `internal/adapters/outbound/spinitron` implements catalog provider ports and hides provider pagination/API payloads;
- `internal/adapters/outbound/email` implements auth email sender ports and hides SendGrid/console delivery details.

Keep concrete adapter construction in `internal/cmd`, the composition root.
