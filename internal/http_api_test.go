package internal_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Muxcore-Media/media-books/internal"
)

func startTestModule(t *testing.T) *internal.Module {
	t.Helper()
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := copyTree(filepath.Join("testdata", "library"), lib); err != nil {
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
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	return m
}

func TestHTTPListAuthorsFixtures(t *testing.T) {
	m := startTestModule(t)

	hr, err := http.Get("http://" + m.HTTPListenAddr() + "/api/authors")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hr.Body.Close() }()
	if hr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(hr.Body)
		t.Fatalf("status %d: %s", hr.StatusCode, b)
	}
	body, err := io.ReadAll(hr.Body)
	if err != nil {
		t.Fatal(err)
	}
	var authors []map[string]any
	if err := json.Unmarshal(body, &authors); err != nil {
		t.Fatal(err)
	}
	if len(authors) < 1 {
		t.Fatal("expected fixture authors via HTTP API")
	}

	booksResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/books")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = booksResp.Body.Close() }()
	if booksResp.StatusCode != http.StatusOK {
		t.Fatalf("books status %d", booksResp.StatusCode)
	}

	missingResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = missingResp.Body.Close() }()
	if missingResp.StatusCode != http.StatusOK {
		t.Fatalf("missing status %d", missingResp.StatusCode)
	}
	var missing struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
	}
	if err := json.NewDecoder(missingResp.Body).Decode(&missing); err != nil {
		t.Fatal(err)
	}
	if missing.Page != 1 || missing.PageSize != 100 {
		t.Fatalf("unexpected pagination: %+v", missing)
	}
}

func TestHTTPScan(t *testing.T) {
	m := startTestModule(t)
	resp, err := http.Post("http://"+m.HTTPListenAddr()+"/api/scan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var res struct {
		FilesFound int `json:"files_found"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.FilesFound < 2 {
		t.Fatalf("expected scanned files: %+v", res)
	}
}

func TestHTTPAuthorDetailAndStream(t *testing.T) {
	m := startTestModule(t)

	listResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/books")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	var books []struct {
		ID       string `json:"id"`
		AuthorID string `json:"author_id"`
		Files    []struct {
			ID        string `json:"id"`
			StreamURL string `json:"stream_url"`
		} `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 || len(books[0].Files) == 0 {
		t.Fatal("expected books with files from startup scan")
	}
	if books[0].Files[0].StreamURL == "" {
		t.Fatal("expected stream_url on list")
	}

	detailResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/authors/" + books[0].AuthorID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = detailResp.Body.Close() }()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("detail status %d", detailResp.StatusCode)
	}
	var detail struct {
		Author map[string]any `json:"author"`
		Books  []struct {
			Files []struct {
				StreamURL string `json:"stream_url"`
			} `json:"files"`
		} `json:"books"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Books) == 0 || len(detail.Books[0].Files) == 0 || detail.Books[0].Files[0].StreamURL == "" {
		t.Fatal("expected stream_url on author detail")
	}

	streamResp, err := http.Get("http://" + m.HTTPListenAddr() + books[0].Files[0].StreamURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = streamResp.Body.Close() }()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", streamResp.StatusCode)
	}
}

func TestHTTPStreamUnknownID404(t *testing.T) {
	m := startTestModule(t)
	resp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/files/bf_nope/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHTTPStreamPathEscape404(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(data, "outside.epub")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := m.Store()
	au, err := s.AddAuthor(t.Context(), internal.Author{Name: "Esc", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(t.Context(), internal.Book{AuthorID: au.ID, Title: "Book", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.UpsertBookFileForTest(t.Context(), internal.BookFile{
		BookID: bk.ID, AuthorID: au.ID, Title: "outside", Path: outside,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	resp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/files/" + f.ID + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for path escape, got %d", resp.StatusCode)
	}
}

func TestHTTPImportBookFile(t *testing.T) {
	m := startTestModule(t)

	listResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/books")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	var books []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 {
		t.Fatal("expected books")
	}

	stub := filepath.Join(m.LibraryDir(), "Fixture Author", "Sample Book", "import.epub")
	if err := os.WriteFile(stub, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": stub})
	resp, err := http.Post(
		"http://"+m.HTTPListenAddr()+"/api/books/"+books[0].ID+"/import",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("import status %d: %s", resp.StatusCode, b)
	}
	var imported struct {
		ID        string `json:"id"`
		StreamURL string `json:"stream_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&imported); err != nil {
		t.Fatal(err)
	}
	if imported.ID == "" || !strings.HasSuffix(imported.StreamURL, "/stream") {
		t.Fatalf("import: %+v", imported)
	}
}
