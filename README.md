# Media Books

Readarr-class **book library manager** scaffold for MuxCore.

Exposes `muxcore.books.v1.BookManagementService` (authors/books) with an in-memory store in **v0.1.0**, plus SettingsProvider for `library_dir`.

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9650` |
| Health | `:9651` |

## Build / test

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o bin/media-books ./cmd/module
```

## Status

v0.1.0 scaffold — optional peer. Persistence, metadata search, and acquisition wiring are follow-ups.
