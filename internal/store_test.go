package internal_test

import (
	"os"
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

func TestNewModuleDefaultLibraryDir(t *testing.T) {
	data := t.TempDir()
	m := internal.NewModule(internal.Config{DataDir: data})
	if m.LibraryDir() != data {
		t.Fatalf("library dir=%q want data dir %q", m.LibraryDir(), data)
	}
}

func TestStoreUpdateAuthorBook(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	au, err := s.AddAuthor(ctx, internal.Author{Name: "Author", GoodreadsID: "1", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Title", ISBN: "978", Year: 2000, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}

	au, err = s.UpdateAuthor(ctx, au.ID, map[string]any{
		"name": "Renamed", "goodreads_id": "2", "path": "/books/author", "monitored": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if au.Name != "Renamed" || au.GoodreadsID != "2" || au.Path != "/books/author" || au.Monitored {
		t.Fatalf("%+v", au)
	}

	bk, err = s.UpdateBook(ctx, bk.ID, map[string]any{
		"title": "New Title", "isbn": "979", "year": int32(2001), "monitored": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bk.Title != "New Title" || bk.ISBN != "979" || bk.Year != 2001 || bk.Monitored {
		t.Fatalf("%+v", bk)
	}

	items, total, err := s.ListMissingBooks(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("unmonitored book should not be missing: total=%d items=%d", total, len(items))
	}
}

func TestStoreRemoveAuthorDeleteFiles(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	lib := t.TempDir()
	bookDir := filepath.Join(lib, "Author", "Book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(bookDir, "book.epub")
	if err := os.WriteFile(stub, []byte("ebook"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.epub")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	au, err := s.AddAuthor(ctx, internal.Author{Name: "Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Book", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBookFileForTest(ctx, internal.BookFile{
		BookID: bk.ID, AuthorID: au.ID, Title: "book", Path: stub,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBookFileForTest(ctx, internal.BookFile{
		BookID: bk.ID, AuthorID: au.ID, Title: "outside", Path: outside,
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveAuthorFiles(ctx, au.ID, lib, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stub); !os.IsNotExist(err) {
		t.Fatalf("expected stub removed, stat err=%v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain: %v", err)
	}
}

func TestStoreRemoveBookDeleteFiles(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	lib := t.TempDir()
	keepStub := filepath.Join(lib, "keep.epub")
	delStub := filepath.Join(lib, "del.epub")
	for _, p := range []string{keepStub, delStub} {
		if err := os.WriteFile(p, []byte("ebook"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	au, err := s.AddAuthor(ctx, internal.Author{Name: "Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	keepBk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Keep", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	delBk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Delete", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBookFileForTest(ctx, internal.BookFile{
		BookID: keepBk.ID, AuthorID: au.ID, Title: "keep", Path: keepStub,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBookFileForTest(ctx, internal.BookFile{
		BookID: delBk.ID, AuthorID: au.ID, Title: "del", Path: delStub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveBookFiles(ctx, keepBk.ID, lib, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keepStub); err != nil {
		t.Fatal("keep-files should not unlink")
	}
	if err := s.RemoveBookFiles(ctx, delBk.ID, lib, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(delStub); !os.IsNotExist(err) {
		t.Fatalf("delete-files should unlink, stat err=%v", err)
	}
}

func TestStoreListMissingBooksVanishedFiles(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	lib := t.TempDir()
	stub := filepath.Join(lib, "gone.epub")
	if err := os.WriteFile(stub, []byte("ebook"), 0o644); err != nil {
		t.Fatal(err)
	}
	au, err := s.AddAuthor(ctx, internal.Author{Name: "Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(ctx, internal.Book{AuthorID: au.ID, Title: "Wanted", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBookFileForTest(ctx, internal.BookFile{
		BookID: bk.ID, AuthorID: au.ID, Title: "gone", Path: stub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stub); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListMissingBooks(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].BookID != bk.ID {
		t.Fatalf("vanished file should count as missing: total=%d items=%+v", total, items)
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
