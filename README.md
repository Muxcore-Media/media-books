# Media Books

Readarr-class **book library manager** for MuxCore (v0.3.0).

SQLite-backed authors/books/files, MediaAdmin integration, operator scan/import/stream HTTP, and full `BookManagementService` gRPC surface.

## Services

| Service | Default | Notes |
|---------|---------|-------|
| gRPC (`BookManagementService`, `MediaAdminService`, Settings) | `:9650` | `BOOKS_GRPC_ADDR` / module default |
| HTTP (health + JSON API) | `:9651` | `MUXCORE_HTTP_ADDR`; `Info().HTTPAddr` is the bound listen address |

## Environment

| Variable | Purpose |
|----------|---------|
| `BOOKS_DATA_DIR` | SQLite + module data (default `./data`) |
| `BOOKS_LIBRARY_DIR` | Ebook library root scanned for files (default `$BOOKS_DATA_DIR`) |
| `MUXCORE_HTTP_ADDR` | HTTP bind address |
| `MUXCORE_INSECURE_DISABLE_TLS` | Dev-only plaintext gRPC |

## gRPC

### `muxcore.books.v1.BookManagementService`

Authors, books, files, missing titles, library scan, and fixture import. Key RPCs: `AddAuthor`, `UpdateAuthor`, `RemoveAuthor`, `AddBook`, `UpdateBook`, `GetBook`, `RemoveBook`, `ScanLibrary`, `ListBookFiles`, `ListMissing`, `ImportBookFile`.

### `muxcore.media.admin.v1.MediaAdminService`

Declared in `muxcore.json`. Features: **missing**. Implements list/get/update/delete/refresh for authors.

## HTTP (health port)

| Path | Purpose |
|------|---------|
| `GET /healthz` | Liveness |
| `GET /api/authors?q=` | JSON author list |
| `GET /api/authors/{id}` | JSON author + books + files + `stream_url` |
| `GET /api/books?author_id=` | JSON book list with files |
| `GET /api/missing` | Monitored books with no on-disk files |
| `POST /api/scan` | Walk `library_dir` into SQLite |
| `POST /api/books/{id}/import` | Attach an existing file under `library_dir` (`{"path":"..."}`) |
| `GET /api/files/{id}/stream` | Serve ebook (path confined to `library_dir`) |

Library layout: `Author/Book Title/file.ext`. Scan and import derive metadata from paths/filenames only — **no paid or network metadata APIs**.

## Data layout

| Path | Purpose |
|------|---------|
| `$BOOKS_DATA_DIR/books.db` | SQLite library (WAL, foreign keys ON) |
| `$BOOKS_LIBRARY_DIR` | Ebook files to scan (defaults to `$BOOKS_DATA_DIR`, not a nested `books/` subfolder) |

## Build / test

```bash
cd media-books
export GOCACHE=/tmp/gocache-media-books
go test ./...
CGO_ENABLED=0 go build -o bin/media-books ./cmd/module
```

Offline fixtures: `internal/testdata/library` (empty `.epub` / `.pdf` stubs).

## Status

v0.3.0 — scan on start + HTTP/gRPC, update author/book, delete-files guards, missing-on-disk purge, import/stream API, MediaAdmin, gRPC integration tests.
