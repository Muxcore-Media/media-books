# Changelog

## [v0.2.0] — 2026-08-10

### Added
- SQLite library persistence (`BOOKS_DATA_DIR` / `books.db`, WAL)
- Library root scan (local ebook stubs; path-derived metadata only)
- Offline fixture tests under `internal/testdata/library`
- Self-hosted CI workflow
- HTTP JSON stubs `GET /api/authors`, `GET /api/authors/{id}`, `GET /api/books` on health port

### Changed
- SettingsProvider exposes `library_dir` and `data_dir`

## [v0.1.0] — 2026-08-10

### Added
- `BookManagementService` (authors/books CRUD)
- In-memory library store
- SettingsProvider (`library_dir`)
- Health `:9651`
