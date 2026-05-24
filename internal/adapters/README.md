# Adapter Organization

Adapters translate between external systems and app-owned ports. They may import
`internal/app` and `internal/domain`, but app and domain packages must not import
adapters.

`internal/adapters/http` is the inbound HTTP adapter:

- parse HTTP requests, cookies, headers, and route params;
- call app-owned use-case interfaces;
- map app errors to HTTP status codes;
- delegate view-model formatting to `internal/adapters/http/presenter` and rendering to `internal/adapters/http/ui`.

Do not add business rules, repository interfaces, or database/provider details to `internal/adapters/http`.

Outbound adapters implement app-owned ports:

- `internal/adapters/sqlite` implements repository ports and keeps sqlc-generated types under `internal/adapters/sqlite/dbgen`;
- `internal/adapters/spinitron` implements catalog provider ports and hides provider pagination/API payloads;
- `internal/adapters/email` implements auth email sender ports and hides SendGrid/console delivery details.

Keep concrete adapter construction in `internal/cmd`, the composition root.
