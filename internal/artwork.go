package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxArtworkBytes = 20 << 20

func (m *Module) getImageDir() string {
	if m.imageDir != "" {
		return m.imageDir
	}
	return filepath.Join(m.dataDir, "images")
}

func artworkURL(httpAddr, relPath string) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	return fmt.Sprintf("http://%s/images/%s", httpAddr, relPath)
}

func extFromFilenameOrMIME(filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		if ext == ".jpeg" {
			return ".jpg"
		}
		return ext
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}

func (m *Module) writeArtworkBytes(itemID, filename, contentType string, data []byte) (relPath, mime string, err error) {
	if itemID == "" {
		return "", "", fmt.Errorf("item id required")
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty artwork data")
	}
	if len(data) > maxArtworkBytes {
		return "", "", fmt.Errorf("artwork exceeds %d bytes", maxArtworkBytes)
	}
	ext := extFromFilenameOrMIME(filename, contentType)
	dir := filepath.Join(m.getImageDir(), itemID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create artwork dir: %w", err)
	}
	relPath = filepath.ToSlash(filepath.Join(itemID, "poster"+ext))
	abs := filepath.Join(m.getImageDir(), filepath.FromSlash(relPath))
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return "", "", fmt.Errorf("write artwork: %w", err)
	}
	mime = contentType
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	return relPath, mime, nil
}

func (m *Module) servableArtworkURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "/images/") {
		return raw
	}
	return artworkURL(m.httpAddr, raw)
}

func mustAuthor(ctx context.Context, store *Store, id string) (*Author, error) {
	if store == nil {
		return nil, fmt.Errorf("store not open")
	}
	return store.GetAuthor(ctx, id)
}

func (m *Module) ReplaceArtwork(stream mediaadminv1.MediaAdminService_ReplaceArtworkServer) error {
	ctx := stream.Context()
	var itemID, filename string
	var buf []byte
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch {
		case msg.GetItemId() != "":
			itemID = msg.GetItemId()
		case msg.GetFilename() != "":
			filename = msg.GetFilename()
		default:
			chunk := msg.GetChunk()
			if len(chunk) == 0 {
				continue
			}
			if len(buf)+len(chunk) > maxArtworkBytes {
				return status.Errorf(codes.InvalidArgument, "artwork exceeds %d bytes", maxArtworkBytes)
			}
			buf = append(buf, chunk...)
		}
	}
	if itemID == "" {
		return status.Error(codes.InvalidArgument, "item_id required")
	}
	if _, err := mustAuthor(ctx, m.store, itemID); err != nil {
		return status.Errorf(codes.NotFound, "%v", err)
	}
	relPath, mime, err := m.writeArtworkBytes(itemID, filename, "", buf)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if _, err := m.store.UpdateAuthor(ctx, itemID, map[string]any{"poster_url": relPath}); err != nil {
		return err
	}
	return stream.SendAndClose(&mediaadminv1.ReplaceArtworkResponse{
		Artwork: &mediaadminv1.ArtworkInfo{
			Id: itemID + "_poster", ItemId: itemID,
			Type: mediaadminv1.ArtworkType_ARTWORK_TYPE_POSTER, Url: artworkURL(m.httpAddr, relPath),
			MimeType: mime,
		},
	})
}
