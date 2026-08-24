package internal_test

import (
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-books/internal"
)

func openTempStore(t *testing.T) (*internal.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "books.db")
	s, err := internal.OpenStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestStoreAuthorBookRoundTrip(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	au, err := s.AddAuthor(ctx, internal.Author{Name: "Ursula K. Le Guin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "A Wizard of Earthsea", Year: 1968, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if bk.Title != "A Wizard of Earthsea" {
		t.Fatalf("%+v", bk)
	}
	listed, err := s.ListBooks(ctx, au.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatal("expected 1 book")
	}
	if err := s.RemoveAuthor(ctx, au.ID); err != nil {
		t.Fatal(err)
	}
	books, err := s.ListBooks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatal("expected books cleared")
	}
}

func TestStoreListMissingBooks(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	au, err := s.AddAuthor(ctx, internal.Author{Name: "Terry Pratchett", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Guards! Guards!", Year: 1989, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Unmonitored", Monitored: false}); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListMissingBooks(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d items=%d", total, len(items))
	}
	if items[0].BookID != missing.ID || items[0].AuthorName != "Terry Pratchett" {
		t.Fatalf("%+v", items[0])
	}
}

func TestStorePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "books.db")
	ctx := t.Context()
	s1, err := internal.OpenStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	au, err := s1.AddAuthor(ctx, internal.Author{Name: "Octavia E. Butler", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Kindred", Year: 1979}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := internal.OpenStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	authors, err := s2.ListAuthors(ctx, "butler")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors=%d", len(authors))
	}
	books, err := s2.ListBooks(ctx, authors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Kindred" {
		t.Fatalf("%+v", books)
	}
}
