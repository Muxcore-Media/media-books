# Changelog

## [0.3.3] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.3.2] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.3.0] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

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
