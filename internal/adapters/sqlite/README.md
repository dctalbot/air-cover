# SQLite Adapter Notes

This package is the SQLite/libSQL implementation of the app-owned repository ports. Keep SQLite-specific behavior here and cover cross-adapter behavior through `internal/app/contracttest`.

SQLite assumptions to isolate before adding another database adapter:

- positional `?` placeholders in SQL statements;
- `LastInsertId` after inserts for users and subrequests;
- `DELETE ... RETURNING` for consuming magic links;
- `ON CONFLICT (email) DO NOTHING` for idempotent imports;
- `AUTOINCREMENT` integer IDs in migrations;
- SQLite/libSQL boolean and timestamp scanning behavior;
- goose migration dialect and the migration file layout under `internal/adapters/sqlite/migrations`.

SQL queries are authored in `internal/adapters/sqlite/queries` and generated with sqlc
into `internal/adapters/sqlite/dbgen`. The generated package is adapter-internal:
repository methods map sqlc rows into domain/app types before returning them.

Keep `internal/adapters/sqlite/schema.sql` in sync with the current goose schema
when migrations change, then run `make generate`.
