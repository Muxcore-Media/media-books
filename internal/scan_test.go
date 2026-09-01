package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-books/internal"
)

// TestScanLibraryRootFixtures walks local stub ebook files under testdata/library.
// Files are empty stubs (no paid metadata APIs).
func TestScanLibraryRootFixtures(t *testing.T) {
	root := filepath.Join("testdata", "library")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture library missing: %v", err)
	}

	s, _ := openTempStore(t)
	ctx := t.Context()
	res, err := s.ScanLibraryRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesFound < 2 {
		t.Fatalf("expected >=2 ebook stubs, found=%d imported=%d skipped=%d",
			res.FilesFound, res.FilesImported, res.FilesSkipped)
	}
	if res.FilesImported < 2 {
		t.Fatalf("expected imports, got %+v", res)
	}

	authors, err := s.ListAuthors(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) < 1 {
		t.Fatal("expected at least one author from fixtures")
	}
	var fixtureAuthor *internal.Author
	for _, a := range authors {
		if a.Name == "Fixture Author" {
			fixtureAuthor = a
			break
		}
	}
	if fixtureAuthor == nil {
		t.Fatalf("Fixture Author not found: %+v", authors)
	}
	books, err := s.ListBooks(ctx, fixtureAuthor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) < 1 {
		t.Fatal("expected book from fixtures")
	}
	files, err := s.ListBookFiles(ctx, books[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("expected epub+pdf stubs, files=%d", len(files))
	}

	// Idempotent rescan should skip already-imported paths.
	res2, err := s.ScanLibraryRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if res2.FilesImported != 0 {
		t.Fatalf("expected no new imports on rescan, got %+v", res2)
	}
	if res2.FilesSkipped < 2 {
		t.Fatalf("expected skips on rescan, got %+v", res2)
	}
}

func TestScanLibraryRootPurgesVanishedFiles(t *testing.T) {
	s, _ := openTempStore(t)
	ctx := t.Context()
	root := t.TempDir()
	authorDir := filepath.Join(root, "Author", "Book")
	if err := os.MkdirAll(authorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(authorDir, "gone.epub")
	if err := os.WriteFile(stub, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScanLibraryRoot(ctx, root); err != nil {
		t.Fatal(err)
	}
	books, err := s.ListBooks(ctx, "")
	if err != nil || len(books) != 1 {
		t.Fatalf("books=%d err=%v", len(books), err)
	}
	files, err := s.ListBookFiles(ctx, books[0].ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%d err=%v", len(files), err)
	}
	if err := os.Remove(stub); err != nil {
		t.Fatal(err)
	}
	res, err := s.ScanLibraryRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesRemoved != 1 {
		t.Fatalf("expected purge, got %+v", res)
	}
	items, total, err := s.ListMissingBooks(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected missing after purge: total=%d items=%d", total, len(items))
	}
}

func TestScanLibraryRootMissing(t *testing.T) {
	s, _ := openTempStore(t)
	_, err := s.ScanLibraryRoot(t.Context(), filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestModuleInitScan(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "books")
	src := filepath.Join("testdata", "library")
	if err := copyTree(src, lib); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir:    data,
		LibraryDir: lib,
		GRPCAddr:   "127.0.0.1:0",
		HTTPAddr:   "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	res, err := m.ScanLibrary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesImported < 2 {
		t.Fatalf("%+v", res)
	}
}

// TestScanConfiguredLibrary scans BOOKS_DATA_DIR / BOOKS_LIBRARY_DIR when RUN_LIBRARY_SCAN=1.
// Used by vault soak scripts (go test -c, run binary on host without Go installed).
func TestScanConfiguredLibrary(t *testing.T) {
	if os.Getenv("RUN_LIBRARY_SCAN") != "1" {
		t.Skip("set RUN_LIBRARY_SCAN=1")
	}
	m := internal.NewModule(internal.Config{
		GRPCAddr: "127.0.0.1:0",
		HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	res, err := m.ScanLibrary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesFound == 0 {
		t.Fatalf("no files found in library root: %+v", res)
	}
	t.Logf("scan: %+v", res)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
