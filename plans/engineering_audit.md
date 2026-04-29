# Air Cover — Engineering Audit Report

> **Scope:** Full codebase review across security, testing, maintainability, tooling, consistency, and extensibility.
> **Original Date:** 2026-04-27
> **Last Updated:** 2026-04-29

---

## Summary Scorecard

| Category | Rating | Notes |
|---|---|---|
| Security | 🟢 Excellent | Rate limiting added; type assertions fixed; email client hardened |
| Testing | 🟡 Good | Coverage up from 59% → 91.5%; 100% gate still failing |
| Maintainability | 🟢 Excellent | Clean structure, good patterns, structured logging |
| Tooling | 🟡 Good | CI has version mismatch; lint action not yet adopted |
| Consistency | 🟡 Good | Minor style inconsistencies remain |
| Extensibility | 🟡 Good | Architecture is solid; a few tight couplings |

---

## Progress Since Last Audit

7 of 20 original issues have been resolved. The remaining 13 are documented below with updated context.

### ✅ Resolved Issues

| # | Issue | Resolution |
|---|---|---|
| 2 | Unsafe type assertion in `GetApp` | Now uses safe `userID, _ := r.Context().Value(UserIDKey).(int)` form |
| 4 | No rate limiting on auth endpoints | `httprate.LimitByIP(5, time.Minute)` applied to `/auth/login` and `/auth/verify` |
| 6 | `http.DefaultClient` used in email sender | `HTTPClient` with 10s timeout injected via `SendGridSender` struct field |
| 7 | Sub-request IDs are base64 tokens | Migrated to autoincrementing integers; code paths cleanly separated |
| 9 | `dev` make target kills port 8080 unconditionally | Now uses `xargs -r kill -TERM` with `|| true` for safe fallback |
| 15 | `noreply@aircover.com` hardcoded | `FromEmail` is now a configurable field in `Config`, passed to `NewSender` |
| 19 | `aircover.db` / `coverage.out` not in `.gitignore` | `.gitignore` now covers `*.out`, `*.db`, `bin`, `tmp`, `coverage.html` |

---

## P0 — Critical (Fix Now)

### 1. Test Coverage Gate Is Still Failing

**File:** All packages
**Category:** Testing

Coverage has improved dramatically from **59.3% → 91.5%**, but the 100% gate still fails. Current per-package coverage:

| Package | Coverage |
|---|---|
| `cmd/aircover` | 100.0% |
| `internal/config` | 100.0% |
| `internal/spinitron` | 99.3% |
| `internal/ui` | 100.0% |
| `internal/logger` | 93.3% |
| `internal/email` | 92.0% |
| `internal/api` | 89.8% |
| `internal/db` | 86.9% |
| `internal/cmd` | 77.8% |
| `internal/models` | [no statements] |

**Recommendation:** Focus on the three lowest-coverage packages:
- `internal/cmd` (77.8%): Test the server startup paths — especially the master email provisioning branches and the `newRouter` construction.
- `internal/db` (86.9%): Add tests for remaining error branches in `DeleteUser` (transaction rollback paths), `UseMagicLink` edge cases, and `CreateUser` `LastInsertId` failures.
- `internal/api` (89.8%): Cover remaining error branches in `GetApp` (show pagination edge cases), `PostUsers` validation paths, and `DeleteUsersId`.

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

### 5. Magic Link Tokens: No Index, No Cleanup

**File:** `internal/db/migrations/2026042600000_initial_schema.sql`
**Category:** Security / Performance

`magic_links.token_hash` is queried on every verify (`WHERE token_hash = ?`) but has no index. At scale this is a full table scan. Additionally, old/expired/used magic links are never cleaned up — the table will grow unbounded.

**Recommendation:**
1. Add a unique index: `CREATE UNIQUE INDEX idx_magic_links_token_hash ON magic_links(token_hash);`
2. Add a scheduled cleanup or prune on login: `DELETE FROM magic_links WHERE expires_at < NOW() OR used_at IS NOT NULL` (or use a goose migration with a trigger).

---

## P2 — Medium Priority

### 8. Template Rendering Errors Are Silent After Header Is Written

**File:** `internal/ui/templates.go`, lines 45–66
**Category:** Reliability / User Experience

```go
w.WriteHeader(http.StatusOK)
if err := authenticatedTmpl.Execute(w, data); err != nil {
    slog.Error("Failed to write response", "error", err)
}
```

`WriteHeader` is called before `Execute` in all three render functions (`RenderUnauthenticated`, `RenderAuthenticated`, `RenderAdmin`). If the template fails mid-render (e.g., a nil pointer in template data), the user receives a 200 with a partially-rendered page and a truncated HTML body. The error is only logged.

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

### 11. CI Does Not Run Linter Reliably

**File:** `.github/workflows/ci.yml`
**Category:** Tooling

`make check` runs `lint test build`, and `ci.yml` runs `make check`. However, the lint step in the Makefile conditionally installs `golangci-lint` differently on non-ARM architectures (line 28–31), which may fail on the `ubuntu-latest` CI runner if the install step doesn't work.

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

The sub-request status is set as the raw string `"open"` in the handler (line 290) and checked with string CSS class names (`status-open`, `status-filled`, `status-cancelled`) in the template. There's no central definition of valid statuses.

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

## P3 — Low Priority / Enhancements

### 16. No `ReadTimeout` or `WriteTimeout` on the HTTP Server

**File:** `internal/cmd/server.go`, lines 128–132
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

### 17. `make start` Fetches `air` From the Internet Every Run

**File:** `Makefile`, line 11
**Category:** Developer Experience / Reproducibility

`go run github.com/air-verse/air@latest` fetches the latest version of `air` on every invocation. This is slow and non-deterministic — `@latest` can introduce breaking changes silently.

**Recommendation:** Pin the version (e.g., `@v1.65.1`) and consider vendoring or caching it via `go install` in the `setup` target.

---

### 18. `make generate` Runs the Server Binary (`doc` subcommand) to Build Docs

**File:** `Makefile`, line 16
**Category:** Tooling / Fragility

```makefile
@go run cmd/aircover/main.go doc > docs/routes.json
```

Building route documentation requires the application to be runnable, which requires a valid `.env` / environment. This couples doc generation to runtime configuration, which will break in clean CI environments.

**Recommendation:** Extract route documentation generation into a standalone Go tool (e.g., `tools/gendocs`) that imports the router construction function directly without requiring a full server startup.

---

### 20. README Is Stale

**File:** `README.md`
**Category:** Documentation

The README references `go run cmd/aircover/main.go` without a subcommand (the app now requires `server`), mentions Go 1.24 when `go.mod` says 1.26, and has an empty "Spinitron Integration" section marked `(WIP)`. It also doesn't mention the admin dashboard, user management features, or the magic link authentication system.

**Recommendation:** Update the README with:
- Correct run command (`make start` or `go run cmd/aircover/main.go server`)
- Correct Go version (1.26)
- Expanded Spinitron section with proxy configuration details
- Summary of authentication system (magic links, session cookies)
- Summary of admin functionality (user management, role-based access)

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
- **Structured logging** with `log/slog` backed by `uber-go/zap` — environment-aware configuration (dev vs prod), no `fmt.Println` leakage.
- **`ReadHeaderTimeout`** on the HTTP server — prevents header-based slowloris attacks.
- **CI enforces `git diff --exit-code`** — ensures codegen artifacts stay committed and up to date.
- **`make check` combines lint + test + build** — a single gate for CI and local verification.
- **Ownership authorization on DELETE** — the handler checks `sr.UserID == userID` before deleting, preventing cross-user data deletion.
- **Per-IP rate limiting on auth** — `httprate.LimitByIP` on login and verify endpoints prevents brute-force and quota exhaustion.
- **Configurable email sender** — `FromEmail` and `SendGridAPIKey` from config; `HTTPClient` injected with timeout; console fallback in non-production.
- **Role-based access control** — `RequireAdmin` middleware protects admin endpoints; self-deletion prevented on user management.
- **Transactional user deletion** — `DeleteUser` cascades cleanup of sessions, magic links, and sub-requests within a transaction.
- **Safe type assertions** — all context value extractions use the comma-ok idiom consistently across handlers.
