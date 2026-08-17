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
	s, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestStoreAuthorBookRoundTrip(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(internal.Author{Name: "Ursula K. Le Guin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(internal.Book{AuthorID: au.ID, Title: "A Wizard of Earthsea", Year: 1968, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if bk.Title != "A Wizard of Earthsea" {
		t.Fatalf("%+v", bk)
	}
	listed, err := s.ListBooks(au.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatal("expected 1 book")
	}
	if err := s.RemoveAuthor(au.ID); err != nil {
		t.Fatal(err)
	}
	books, err := s.ListBooks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatal("expected books cleared")
	}
}

func TestStorePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "books.db")
	s1, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	au, err := s1.AddAuthor(internal.Author{Name: "Octavia E. Butler", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.AddBook(internal.Book{AuthorID: au.ID, Title: "Kindred", Year: 1979}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	authors, err := s2.ListAuthors("butler")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors=%d", len(authors))
	}
	books, err := s2.ListBooks(authors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Kindred" {
		t.Fatalf("%+v", books)
	}
}
