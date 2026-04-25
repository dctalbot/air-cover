# Magic Link Authentication Plan

This plan details the implementation of a passwordless authentication system using magic links for the Air Cover application. 

The system will allow an initially configured `MASTER_EMAIL` to log in without a password, and then invite subsequent users.

## User Review Required

> [!TIP]  
> I have updated the plan with your choices (SQLite + stdlib, Sendgrid/Console for email) and provided a recommendation for Session Management below. If this looks good to you, please approve so we can begin execution!

## Decisions Made & Recommendations

- **Database Stack:** `database/sql` with SQLite (for local dev) and Turso (libsql) for production. Zero ORMs to minimize dependencies.
- **Email Provider:** SendGrid for production, console logging for local development.
- **Session Management (Recommended):** **Server-Side Sessions**. Since you want to minimize dependencies, using a server-side `sessions` table is recommended over JWTs. JWTs require external libraries to handle signing and validation securely. A `sessions` table allows us to use Go's standard `crypto/rand` to generate a secure random session ID, store it in an HttpOnly cookie, and map it to the user in the database. It also gives us the ability to instantly revoke access if needed.

## Proposed Changes

### 1. Configuration Management
Add the necessary environment variables to our configuration.

#### [MODIFY] `internal/config/config.go`
- Add `MasterEmail` and `SendGridAPIKey` to the `Config` struct.
- Read them from the environment variables (`MASTER_EMAIL`, `SENDGRID_API_KEY`).

#### [MODIFY] `.env.example`
- Add `MASTER_EMAIL=` and `SENDGRID_API_KEY=` to the example file.

---

### 2. Database Layer (`database/sql`)
Initialize SQLite and create the repository layer. We will use `github.com/mattn/go-sqlite3` or `github.com/tursodatabase/go-libsql` depending on standard usage for Turso. For standard compatibility, `github.com/tursodatabase/libsql-client-go/libsql` provides the `libsql` driver which works for both local SQLite files and remote Turso.

#### [NEW] `internal/db/db.go`
- Provide an `InitDB(uri string)` function that returns a `*sql.DB`.
- Run lightweight schema creation if tables don't exist.

#### [NEW] `internal/db/schema.sql` (or embedded strings in `db.go`)
- `users` table: `id`, `email`, `created_at`.
- `magic_links` table: `id`, `user_id`, `token_hash`, `expires_at`, `used_at`.
- `sessions` table: `id`, `user_id`, `session_token`, `expires_at`.

#### [NEW] `internal/db/repository.go`
- Simple queries: `GetUserByEmail`, `CreateUser`, `CreateMagicLink`, `GetValidMagicLink`, `CreateSession`, `GetSession`.

---

### 3. Application Initialization & Idempotency
Ensure the `MASTER_EMAIL` is set up on boot.

#### [MODIFY] `internal/cmd/server.go`
- Initialize the database connection.
- Check if `cfg.MasterEmail` is set.
- If it doesn't exist in the `users` table, insert it to guarantee the master admin can always access the system.

---

### 4. Email Service Layer
Abstract the email sending logic so we can swap between development and production.

#### [NEW] `internal/email/sender.go`
- Create an interface `Sender` with `SendMagicLink(toEmail, magicLink string) error`.
- Create `ConsoleSender` implementation that just `slog.Info`s the magic link.
- Create `SendGridSender` implementation that uses the SendGrid API (can just use standard `net/http` to POST to SendGrid's API to avoid extra dependencies).
- Initialize the correct implementation based on `cfg.ENV`.

---

### 5. Authentication Flow (Web Handlers)
Implement the routes for requesting and verifying magic links.

#### [NEW] `internal/api/auth.go`
- **`POST /auth/login`**: 
  - Accepts an email address.
  - Queries `users` for the email.
  - Generates a secure random token (`crypto/rand`), hashes it, stores in `magic_links`.
  - Triggers the `email.Sender` to send the link: `https://<domain>/auth/verify?token=<raw_token>`.
- **`GET /auth/verify`**:
  - Accepts the `token` parameter.
  - Hashes it and looks it up in `magic_links`.
  - If valid: generates a new `session_token` via `crypto/rand`, stores it in `sessions`.
  - Sets an `HttpOnly`, `Secure` cookie `session_id=<session_token>`.
  - Redirects to `/`.
- **Middleware `AuthMiddleware`**: Reads the cookie, looks up the session in the database, and injects the `user_id` into the request context.

---

### 6. Invitation System
Allow authenticated users to invite others.

#### [NEW] `internal/api/users.go`
- **`POST /api/users/invite`**:
  - Wrapped in `AuthMiddleware`.
  - Accepts an email address.
  - Inserts a new `user` record.

## Verification Plan

### Automated Tests
- Unit tests for the authentication handlers (`/auth/login` and `/auth/verify`).
- Mock the `email.Sender` to ensure the correct link is generated.
- Mock the database or use an in-memory SQLite DB (`file::memory:?cache=shared`) to test the idempotent `MASTER_EMAIL` creation and session creation logic.

### Manual Verification
- Start the server with `MASTER_EMAIL` set in `.env` and `DB_URI=file:local.db`.
- Check the console logs for the magic link after hitting `/auth/login`.
- Verify that a session cookie is set and access to protected routes is granted.
