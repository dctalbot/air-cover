# Air Cover — Engineering Audit Report

> **Scope:** Full codebase review across security, testing, maintainability, tooling, consistency, and extensibility.
> **Date:** 2026-04-27

---

## Summary Scorecard

| Category | Rating | Notes |
|---|---|---|
| Security | 🟡 Good | Strong foundations; a few gaps |
| Testing | 🔴 Needs Work | Coverage gate failing; gaps in critical paths |
| Maintainability | 🟢 Excellent | Clean structure, good patterns |
| Tooling | 🟡 Good | CI is minimal; lint is well-configured |
| Consistency | 🟡 Good | Minor style inconsistencies |
| Extensibility | 🟡 Good | Architecture is solid; a few tight couplings |

---

## P0 — Critical (Fix Now)

### 1. Test Coverage Gate Is Currently Failing

**File:** All packages
**Category:** Testing

The `make check` command exits with code 2. Overall coverage is **59.3%** against a **100% requirement**. The `internal/api` package is at 47.3%, `internal/email` is at 20%, and `internal/models` has no test files at all.

> [!CAUTION]
> The CI gate (`make check` → `make test`) will block every PR until this is resolved. The 100% coverage requirement is a strong project standard worth keeping, but it's currently unenforceable because the baseline is broken.

**Recommendation:** Bring all packages to 100% coverage. Priority packages:
- `internal/api`: Add tests for the `GetApp` error path when `UserIDKey` is missing from context (the `.(int)` assertion panics silently), and the `showMap` construction with non-integer show IDs that fail `strconv.Atoi`.
- `internal/email`: `SendGridSender.SendMagicLink` has nearly no test coverage. Add a mock `http.Client` to test the happy path, non-2xx responses, and the marshal/request-creation error paths.
- `internal/models`: Add at minimum a compile-time struct sanity test or remove the package from the coverage calculation (via build tags) if it's intentionally model-only.

---

### 2. Unsafe Type Assertion in Hot Path

**File:** `internal/api/handler.go`, line 131
**Category:** Security / Reliability

```go
CanDelete: sr.UserID == r.Context().Value(UserIDKey).(int),
```

This is an **unguarded type assertion**. If `UserIDKey` is absent or has a different type in context (e.g., during testing or middleware changes), this panics and brings down the server. The `DeleteSubRequestsId` handler correctly uses the two-return form `(int, ok)` on line 247, but `GetApp` does not.

**Recommendation:** Use the safe form:
```go
userID, _ := r.Context().Value(UserIDKey).(int)
CanDelete: sr.UserID == userID,
```

---

## P1 — High Priority

### 3. No Graceful Shutdown

**File:** `internal/cmd/server.go`
**Category:** Maintainability / Reliability

The server starts with `listenAndServe(server)` but there is no signal handling (`SIGTERM`, `SIGINT`) and no call to `server.Shutdown()`. In production (e.g., on Fly.io or Railway), a `SIGTERM` will kill in-flight requests abruptly.

**Recommendation:** Wrap the server start in a goroutine and listen for OS signals:
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
go func() {
    if err := listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
        slog.Error("Server failed", "error", err)
        osExit(1)
    }
}()
<-quit
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
server.Shutdown(ctx)
```

---

### 4. No Rate Limiting on Auth Endpoints

**File:** `internal/cmd/server.go`, `internal/api/auth.go`
**Category:** Security

The `/auth/login` endpoint accepts unlimited requests per IP. A bad actor can enumerate valid emails (even though the response is identical, timing differences can leak information) and flood the `magic_links` table and SendGrid quota.

**Recommendation:** Apply per-IP rate limiting using `go-chi/httprate` or a simple token-bucket middleware. At minimum, protect `POST /auth/login` and `GET /auth/verify`. This is a 1-import, ~10-line change with `chi`.

```go
import "github.com/go-chi/httprate"

r.With(httprate.LimitByIP(5, time.Minute)).Post("/auth/login", ...)
```

---

### 5. Magic Link Tokens Stored as Plain-Text Hash, But No Index

**File:** `internal/db/migrations/2026042600000_initial_schema.sql`
**Category:** Security / Performance

`magic_links.token_hash` is queried on every verify (`WHERE token_hash = ?`) but has no index. At scale this is a full table scan. Additionally, old/expired/used magic links are never cleaned up — the table will grow unbounded.

**Recommendation:**
1. Add a unique index: `CREATE UNIQUE INDEX idx_magic_links_token_hash ON magic_links(token_hash);`
2. Add a scheduled cleanup or prune on login: `DELETE FROM magic_links WHERE expires_at < NOW() OR used_at IS NOT NULL` (or use a goose migration with a trigger).

---

### 6. `http.DefaultClient` Used in Email Sender

**File:** `internal/email/sender.go`, line 66
**Category:** Security / Reliability

```go
resp, err := http.DefaultClient.Do(req)
```

`http.DefaultClient` has no timeout, meaning an unresponsive SendGrid API will hang a goroutine indefinitely (and block the login response to the user).

**Recommendation:** Use a client with a timeout:
```go
httpClient: &http.Client{Timeout: 10 * time.Second}
```
Inject it via the struct field rather than using the global default. This also improves testability.

---

### 7. Sub-Request IDs Are Not UUIDs — They're Base64 Tokens

**File:** `internal/api/handler.go`, line 205; `internal/api/auth.go`, line 32
**Category:** Consistency / Security

Sub-request IDs are generated with `generateRandomToken(32)` which produces base64-URL-encoded random bytes. While cryptographically strong, these are longer than needed for a primary key and share a code path with auth tokens — conceptually different concerns.

**Recommendation:** Use `github.com/google/uuid` (already a transitive dependency) for entity IDs:
```go
import "github.com/google/uuid"
id := uuid.New().String()
```
This is shorter, more recognizable, and clearly distinguishes "identity" from "secret token."

---

## P2 — Medium Priority

### 8. Template Rendering Errors Are Silent After Header Is Written

**File:** `internal/ui/templates.go`, lines 36–46
**Category:** Reliability / User Experience

```go
w.WriteHeader(http.StatusOK)
if err := authenticatedTmpl.Execute(w, data); err != nil {
    slog.Error("Failed to write response", "error", err)
}
```

`WriteHeader` is called before `Execute`. If the template fails mid-render (e.g., a nil pointer in template data), the user receives a 200 with a partially-rendered page and a truncated HTML body. The error is only logged.

**Recommendation:** Render templates into a `bytes.Buffer` first, then write:
```go
var buf bytes.Buffer
if err := authenticatedTmpl.Execute(&buf, data); err != nil {
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    return
}
w.Header().Set("Content-Type", "text/html; charset=utf-8")
_, _ = buf.WriteTo(w)
```

---

### 9. `dev` Make Target Kills Port 8080 Unconditionally

**File:** `Makefile`, line 13
**Category:** Developer Experience / Safety

```makefile
dev:
    kill -9 $$(lsof -t -i :8080)
```

`kill -9` on an empty result causes `make` to print a kill error. More dangerously, if another developer's unrelated process happens to be on port 8080 (e.g., another project's dev server), it is killed without warning. `kill -9` also bypasses graceful shutdown.

**Recommendation:**
```makefile
dev:
    @lsof -ti :8080 | xargs -r kill -TERM 2>/dev/null || true
    go run github.com/air-verse/air@latest -c .air.toml
```

---

### 10. CI Workflow Has a Go Version Mismatch

**File:** `.github/workflows/ci.yml`, line 18; `go.mod`, line 3
**Category:** Tooling / Reliability

`ci.yml` specifies `go-version: '1.25.x'` but `go.mod` declares `go 1.26`. These are out of sync. If 1.26 uses language features not in 1.25, CI will fail (or silently build with the wrong semantics).

**Recommendation:** Pin the CI Go version to match `go.mod`:
```yaml
go-version: '1.26.x'
```
Or use `go-version-file: go.mod` to automatically track it.

---

### 11. CI Does Not Run Linter

**File:** `.github/workflows/ci.yml`
**Category:** Tooling

`make check` runs `lint test build`, and `ci.yml` runs `make check`. However, the lint step in the Makefile conditionally installs `golangci-lint` differently on non-ARM architectures (line 22–24), which may fail on the `ubuntu-latest` CI runner if the install step doesn't work.

**Recommendation:** Use the official `golangci-lint-action` in CI for reliable linting, independent of the local install script:
```yaml
- uses: golangci/golangci-lint-action@v8
  with:
    version: v2.11.4
```

---

### 12. Repository Layer Accepts `*sql.DB` Directly — No Abstraction for Testing

**File:** `internal/db/repository.go`
**Category:** Testability / Extensibility

`Repository` holds a `*sql.DB` (concrete type). To test it, every test must spin up a real SQLite database. While the tests do this correctly (using `:memory:`), there's no interface for mocking the repository in handler tests.

The handler tests in `internal/api/handler_test.go` also use `setupTestDB` (a real SQLite connection). This means API-layer tests are actually integration tests, making them slower and more fragile than necessary.

**Recommendation:** Define a `Storer` interface in the `api` package (or a shared `store` package):
```go
type Storer interface {
    GetUserByEmail(ctx context.Context, email string) (*models.User, error)
    CreateSubRequest(ctx context.Context, sr *models.SubRequest) error
    // ...
}
```
Then `Server` takes a `Storer` instead of `*db.Repository`. This enables fast in-memory mock tests for handlers without hitting SQLite.

---

### 13. `SubstituteID` Field in Model Is Unused

**File:** `internal/models/models.go`, line 20
**Category:** Maintainability

```go
SubstituteID *int `json:"substitute_id"` // Persona ID, nil if not picked up yet
```

This field exists in the model but is absent from the `sub_requests` DB schema and never populated or used in any handler or query. Dead fields in models are confusing and can mislead future contributors.

**Recommendation:** Either add it to the schema and implement the "pick up a sub request" feature, or remove it from the model until that feature is built.

---

### 14. `Status` Field Uses Magic Strings

**File:** `internal/models/models.go`, `internal/api/handler.go`
**Category:** Consistency / Maintainability

The sub-request status is set as the raw string `"open"` in the handler and checked with string CSS class names (`status-open`, `status-filled`, `status-cancelled`) in the template. There's no central definition of valid statuses.

**Recommendation:** Define a typed constant set in `models`:
```go
type SubRequestStatus string

const (
    StatusOpen      SubRequestStatus = "open"
    StatusFilled    SubRequestStatus = "filled"
    StatusCancelled SubRequestStatus = "cancelled"
)
```
This enables compile-time safety, makes valid values self-documenting, and integrates cleanly with the `validate` struct tag.

---

### 15. `noreply@aircover.com` is Hardcoded in Email Sender

**File:** `internal/email/sender.go`, line 42
**Category:** Maintainability / Configurability

```go
"email": "noreply@aircover.com", // This should probably be configurable too
```

The comment itself acknowledges this is a problem. A hardcoded from-address breaks deployments that use a different domain.

**Recommendation:** Add `FromEmail string` to the `Config` struct and pass it into `NewSender`.

---

## P3 — Low Priority / Enhancements

### 16. No `ReadTimeout` or `WriteTimeout` on the HTTP Server

**File:** `internal/cmd/server.go`, lines 116–120
**Category:** Security / Reliability

Only `ReadHeaderTimeout` is set. A slow client can keep a connection alive indefinitely by sending body data slowly (slowloris-style on write, or slow reads).

**Recommendation:**
```go
server := &http.Server{
    Addr:              ":" + portStr,
    Handler:           r,
    ReadHeaderTimeout: 3 * time.Second,
    ReadTimeout:       15 * time.Second,
    WriteTimeout:      15 * time.Second,
    IdleTimeout:       60 * time.Second,
}
```

---

### 17. `make dev` Fetches `air` From the Internet Every Run

**File:** `Makefile`, line 14
**Category:** Developer Experience / Reproducibility

`go run github.com/air-verse/air@latest` fetches the latest version of `air` on every invocation. This is slow and non-deterministic — `@latest` can introduce breaking changes silently.

**Recommendation:** Pin the version (e.g., `@v1.61.7`) and consider vendoring or caching it via `go install` in the `setup` target.

---

### 18. `make generate` Runs the Server Binary (`doc` subcommand) to Build Docs

**File:** `Makefile`, line 19
**Category:** Tooling / Fragility

```makefile
go run cmd/aircover/main.go doc > docs/routes.json
```

Building route documentation requires the application to be runnable, which requires a valid `.env` / environment. This couples doc generation to runtime configuration, which will break in clean CI environments.

**Recommendation:** Extract route documentation generation into a standalone Go tool (e.g., `tools/gendocs`) that imports the router construction function directly without requiring a full server startup.

---

### 19. `aircover.db` and `coverage.out` Are Tracked by Git (Potentially)

**File:** `.gitignore`
**Category:** Developer Experience

`aircover.db` (the local SQLite file) and `coverage.out` (test artifact) should be in `.gitignore`. If they're being committed, it leaks local test data and creates noisy diffs.

**Recommendation:** Verify `.gitignore` includes:
```
aircover.db
coverage.out
bin/
tmp/
```

---

### 20. README Is Stale

**File:** `README.md`
**Category:** Documentation

The README references `go run cmd/aircover/main.go` without a subcommand (the app now requires `server`), mentions Go 1.24 when `go.mod` says 1.26, and has an empty "Spinitron Integration" section marked `(WIP)`.

**Recommendation:** Update the README with the correct run command (`make start` or `go run cmd/aircover/main.go server`), the correct Go version, and expand the Spinitron section with the proxy configuration details.

---

## What's Working Well ✅

These patterns are exemplary and should be preserved:

- **Schema-first API design** via `api/openapi.yaml` + `oapi-codegen` codegen — a mature, contract-driven approach.
- **Embedded migrations** with `goose` and `//go:embed` — migrations are compiled into the binary and run automatically on startup, preventing schema drift.
- **Embedded HTML templates** via `//go:embed` — templates are part of the binary; no runtime file-system dependency.
- **Config centralization** — `forbidigo` lint rule blocks direct `os.Getenv` calls outside the `config` package. This is an excellent enforcement of a clean configuration boundary.
- **Token hashing** — magic link tokens are hashed (SHA-256) before storage. The raw token is never persisted, following secure token storage best practices.
- **`HttpOnly` + `SameSite` cookies** — session cookies are correctly hardened.
- **Request body size limit** — `http.MaxBytesReader` is applied on form-parsing handlers.
- **Structured logging** with `log/slog` throughout — no `fmt.Println` leakage in production code.
- **`ReadHeaderTimeout`** on the HTTP server — prevents header-based slowloris attacks.
- **CI enforces `git diff --exit-code`** — ensures codegen artifacts stay committed and up to date.
- **`make check` combines lint + test + build** — a single gate for CI and local verification.
- **Ownership authorization on DELETE** — the handler checks `sr.UserID == userID` before deleting, preventing cross-user data deletion.
