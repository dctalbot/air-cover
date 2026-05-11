# Improve Test Coverage to 100%

Current overall coverage is 94.2%. The goal is 100%. The Makefile already filters out `.gen.go` and `_templ.go` from coverage, so we only need to cover hand-written code.

## Coverage Gap Analysis

Based on `go tool cover -func=coverage.filtered.out`, here are the specific uncovered lines:

### `internal/ui` — 58.4% (biggest gap)

The `_templ.go` files are excluded from coverage, but `viewmodels.go` (100% trivially — just struct definitions) is included. **The 58.4% number is misleading** — it likely counts generated `_templ.go` code at per-package level even though it's filtered at the aggregate level. The existing tests in `ui_test.go` already cover all templ components. No action needed here unless per-package coverage is also enforced.

> [!IMPORTANT]
> The `make test` target only checks **aggregate** coverage from `coverage.filtered.out`, not per-package. The aggregate is 94.2%. The per-package 58.4% for `internal/ui` doesn't affect the check. We only need to bring the aggregate from 94.2% → 100%.

### `internal/api` — 88.2% (aggregate impact)

| Function | Coverage | Missing |
|---|---|---|
| `generateRandomToken` | 75% | Error path (rand.Read failure) |
| `HandleLogin` | 88.2% | `CreateMagicLink` error path (line ~103-107) |
| `HandleVerify` | 66.7% | `generateRandomToken` error in session ID/token generation (lines ~153-165) |
| `Get` | 85.7% | `Unauthenticated().Render()` error path (line 59-61) |
| `GetApp` | 97.3% | `Authenticated().Render()` error path (line 131-133) |
| `GetAdmin` | 83.3% | `ListUsers` DB error, `Admin().Render()` error path |
| `DeleteSubRequestsId` | 85% | `DeleteSubRequest` DB error path (line 321-325) |
| `PatchUsersId` | 91.7% | `UpdateUser` not-found path, missing UserIDKey |

### `internal/cmd` — 92.7%

| Function | Coverage | Missing |
|---|---|---|
| `runMigrate` | 85.7% | `sql.Open` error path (line ~62-65) |
| `newRouter` | 93.3% | `GetSwagger` error → osExit path (line ~37-40) |

### `internal/db` — 92.6%

| Function | Coverage | Missing |
|---|---|---|
| `InitDB` | 75% | `db.Ping()` error path |
| `RunMigration` | 92.3% | `goose.SetDialect` error (unlikely but possible) |
| `CreateUser` | 85.7% | `LastInsertId` error |
| `UseMagicLink` | 86.7% | `usedAt.Valid` branch where link was already used |
| `CreateSubRequest` | 87.5% | `LastInsertId` error |
| `ListSubRequests` | 92.9% | `rows.Err()` error |
| `DeleteSubRequest` | 88.9% | `RowsAffected` error |
| `ListUsers` | 92.3% | `rows.Err()` error |
| `UpdateUser` | 95.5% | `RowsAffected` error |
| `ImportUsers` | 93.8% | `stmt.ExecContext` error |

### `internal/email` — 92.0%

| Function | Coverage | Missing |
|---|---|---|
| `SendMagicLink` (SendGrid) | 90% | `json.Marshal` error path (line ~57-59) |

### `internal/logger` — 93.3%

| Function | Coverage | Missing |
|---|---|---|
| `NewLogger` | 93.3% | `zcfg.Build()` error fallback path (line ~31-32) |

### `internal/spinitron` — 99.4%

| Function | Coverage | Missing |
|---|---|---|
| `get` | 95% | `io.ReadAll` error path or `http.NewRequestWithContext` error |

## Proposed Changes

Given the difficulty of testing some of these error paths (e.g., `rand.Read` failure, `json.Marshal` on a simple map, `zcfg.Build()` error), the most pragmatic approach is to:

1. **Add tests for straightforwardly testable paths** (most DB error paths, handler error paths)
2. **Use testable abstractions** for paths that need dependency injection (e.g., closed DB connections)
3. **Skip truly untestable paths** by recognizing that some error branches (like `rand.Read` failing) are not practically reachable

> [!IMPORTANT]
> Many of the uncovered lines are "defensive error handling" for situations like crypto random failure, JSON marshal failure on a known-good type, or zap config build failure. These are practically impossible to trigger in tests without mocking at a very low level. The **pragmatic strategy** is to focus on the testable paths first and see how close we get.

---

### `internal/api` Tests

#### [MODIFY] [auth_test.go](file:///Users/dctalbot/Developer/air-cover/internal/api/auth_test.go)
- Add test for `HandleLogin` when `CreateMagicLink` fails (close DB after user lookup succeeds but before magic link creation — tricky, but can be done by manipulating the DB between steps)
- Add test for `HandleVerify` where `CreateSession` fails (need magic link to succeed but session to fail)
- Add test for `PatchUsersId` unauthorized (no `UserIDKey` in context)
- Add test for `PatchUsersId` invalid role
- Add test for `PatchUsersId` user not found
- Add test for `GetAdmin` DB error path

#### [MODIFY] [handler_test.go](file:///Users/dctalbot/Developer/air-cover/internal/api/handler_test.go)
- Add test for `GetAdmin` when `ListUsers` fails (closed DB)
- Add test for `DeleteSubRequestsId` when `DeleteSubRequest` itself fails (not GetSubRequestByID)
- Add test for render error paths using an error-producing `ResponseWriter`

---

### `internal/cmd` Tests

#### [MODIFY] [server_test.go](file:///Users/dctalbot/Developer/air-cover/internal/cmd/server_test.go)
- Add test for `newRouter` when `GetSwagger` fails (mock `api.GetSwagger` if possible, or test the osExit path)
- Add test for `runMigrate` when `sql.Open` returns an error

---

### `internal/db` Tests

#### [MODIFY] [db_test.go](file:///Users/dctalbot/Developer/air-cover/internal/db/db_test.go)
- Add test for `UseMagicLink` when link has been used (set `used_at` column directly)
- Add tests for `LastInsertId` error in `CreateUser` and `CreateSubRequest` (difficult without mocking `sql.Result`)
- Add test for `RowsAffected` error paths in `DeleteSubRequest` and `UpdateUser`
- Add test for `rows.Err()` in `ListSubRequests` and `ListUsers`
- Add test for `ImportUsers` exec error (insert a row that violates constraints after prepare succeeds)

---

### `internal/email` Tests

#### [MODIFY] [sender_test.go](file:///Users/dctalbot/Developer/air-cover/internal/email/sender_test.go)
- The `json.Marshal` error path on line 57-59 is practically impossible to trigger since the payload is a `map[string]interface{}` with only strings. We may need to skip this or accept it's unreachable.

---

### `internal/logger` Tests

#### [MODIFY] [logger_test.go](file:///Users/dctalbot/Developer/air-cover/internal/logger/logger_test.go)
- The `zcfg.Build()` error path is extremely unlikely with valid configs. Difficult to test without patching `zap.Config`.

---

### `internal/spinitron` Tests

- The `get()` function at 95% has one or two uncovered error branches. Need to check which specific one.

---

## Verification Plan

### Automated Tests
```bash
make check
```

This runs linting, all tests with coverage, and verifies 100% coverage on filtered output.

## Open Questions

1. **How strict is the 100% requirement?** Some error paths (like `rand.Read` failure, `json.Marshal` on a simple map) are essentially untestable without mocking low-level stdlib functions. Should we restructure code to make these testable (e.g., inject random reader), or is near-100% acceptable?
2. **Should we skip `internal/ui` per-package coverage?** The `_templ.go` files inflate the uncovered count at the per-package level, but the Makefile already filters them from the aggregate check.
