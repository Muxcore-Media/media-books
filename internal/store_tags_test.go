package internal

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAuthorTagsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := OpenStore(ctx, filepath.Join(t.TempDir(), "books.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	au, err := s.AddAuthor(ctx, Author{Name: "Tagged Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateTag(ctx, "favorites")
	if err != nil || id == "" {
		t.Fatalf("create tag: %v %q", err, id)
	}
	if err := s.SetItemTags(ctx, au.ID, []string{id}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetItemTags(ctx, au.ID)
	if err != nil || len(got) != 1 || got[0].Label != "favorites" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
