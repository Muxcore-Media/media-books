# Changelog

## [v0.3.0] — 2026-08-31

### Added
- `ScanLibrary` gRPC + `POST /api/scan`; startup scan in `Module.Start`
- `UpdateAuthor` / `UpdateBook` gRPC + store support (monitored toggle fixes `/api/missing`)
- `GetBook` / `RemoveBook` RPC; `delete_files` honors `library_dir` root
- `ListBookFiles` / `ListMissing` gRPC; `files` on proto `Book`
- `ImportBookFile` gRPC + `POST /api/books/{id}/import` (fixture paths under `library_dir`)
- Hardened `GET /api/files/{id}/stream` via `GetBookFile` + path guard
- `MediaAdminService` (`contracts-media-admin`) with missing feature
- gRPC integration tests; expanded HTTP/store/scan tests
- `PRAGMA foreign_keys=ON`; `Health()` pings SQLite

### Changed
- Default `BOOKS_LIBRARY_DIR` to `BOOKS_DATA_DIR` (no nested `$DATA/books/books`)
- `Info().HTTPAddr` reports HTTP listen address (not gRPC)
- Settings `library_dir` mkdir+validate on update

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
