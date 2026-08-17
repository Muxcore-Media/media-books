# Build from MuxCore workspace root:
#   docker build -f media-books/Dockerfile .
FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY media-books/ /build/media-books/
WORKDIR /build/media-books
RUN go mod download && CGO_ENABLED=0 go build -o /media-books ./cmd/module
FROM alpine:3.21
RUN adduser -D -h /data app
USER app
WORKDIR /app
COPY --from=builder /media-books .
ENTRYPOINT ["./media-books"]
