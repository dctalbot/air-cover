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
