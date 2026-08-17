# Media Books

Readarr-class **book library manager** for MuxCore.

Exposes `muxcore.books.v1.BookManagementService` (authors/books CRUD) with **SQLite persistence** under the data directory, plus SettingsProvider for `library_dir` / `data_dir`.

Library scanning walks the configured root for local ebook stubs (`.epub`, `.pdf`, …) and upserts authors/books/files from the path layout `Author/Book Title/file.ext`. Metadata is derived from filenames only — **no paid or network metadata APIs**.

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9650` |
| Health | `:9651` (`/healthz`, JSON stubs `GET /api/authors`, `GET /api/authors/{id}`, `GET /api/books`) |

## Data layout

| Path | Purpose |
|------|---------|
| `$BOOKS_DATA_DIR/books.db` (default `./data/books.db`) | SQLite library |
| `$BOOKS_LIBRARY_DIR` (default `./data/books`) | Ebook files to scan |

## Build / test

```bash
export PATH=$HOME/.local/go/bin:$PATH
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o bin/media-books ./cmd/module
```

Offline tests use `internal/testdata/library` (empty `.epub` / `.pdf` stubs).

## Status

v0.2.0 — SQLite persistence + library root scan fixtures. Metadata search and acquisition wiring remain follow-ups.
