# Adapter Organization

`internal/api` is currently the HTTP adapter. It remains in its original package to avoid churn around generated OpenAPI code, but it should be treated as an outer adapter:

- parse HTTP requests, cookies, headers, and route params;
- call app-owned use-case interfaces;
- map app errors to HTTP status codes;
- delegate view-model formatting to `internal/presenter` and rendering to `internal/ui`.

Do not add business rules, repository interfaces, or database/provider details to `internal/api`. Once generated-code paths are worth changing, moving it to `internal/adapters/http` should be a mechanical import-path change rather than a behavioral migration.
