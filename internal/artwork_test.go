package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
)

func TestWriteAndListAuthorArtwork(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	m := NewModule(Config{DataDir: data, LibraryDir: filepath.Join(data, "books"), GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	au, err := m.store.AddAuthor(ctx, Author{Name: "Poster Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	rel, _, err := m.writeArtworkBytes(au.ID, "cover.jpg", "image/jpeg", []byte("fakepng"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.store.UpdateAuthor(ctx, au.ID, map[string]any{"poster_url": rel}); err != nil {
		t.Fatal(err)
	}
	resp, err := m.ListArtwork(ctx, &mediaadminv1.ListArtworkRequest{Id: au.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetArtwork()) != 1 || !strings.Contains(resp.GetArtwork()[0].GetUrl(), "/images/"+au.ID+"/poster.jpg") {
		t.Fatalf("artwork=%+v", resp.GetArtwork())
	}
	if _, err := os.Stat(filepath.Join(m.getImageDir(), filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}
