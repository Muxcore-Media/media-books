# Compatibility

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.2.0         | 0.5.2+      | Current |
| v0.1.0         | 0.5.2+      | Superseded |

## Capabilities

- `media.books` / `books`
- `settings`

## Persistence

- SQLite via `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`)
- Default DB: `$BOOKS_DATA_DIR/books.db`
- Library scan: local path/filename only (no paid metadata APIs)
