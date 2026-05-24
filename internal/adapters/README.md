# Adapter Organization

`internal/adapters/http` is the HTTP adapter:

- parse HTTP requests, cookies, headers, and route params;
- call app-owned use-case interfaces;
- map app errors to HTTP status codes;
- delegate view-model formatting to `internal/adapters/http/presenter` and rendering to `internal/adapters/http/ui`.

Do not add business rules, repository interfaces, or database/provider details to `internal/adapters/http`.
