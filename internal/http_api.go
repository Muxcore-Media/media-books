package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (m *Module) registerBooksHTTPAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", m.handleListAuthorsHTTP)
	mux.HandleFunc("GET /api/authors/{id}", m.handleGetAuthorHTTP)
	mux.HandleFunc("GET /api/books", m.handleListBooksHTTP)
	mux.HandleFunc("GET /api/missing", m.handleListMissingHTTP)
	mux.HandleFunc("POST /api/scan", m.handleScanHTTP)
	mux.HandleFunc("POST /api/books/{id}/import", m.handleImportBookHTTP)
	mux.HandleFunc("GET /api/files/{id}/stream", m.handleStreamBookFileHTTP)
}

func (m *Module) handleListAuthorsHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListAuthors(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]authorJSON, 0, len(items))
	for _, a := range items {
		out = append(out, toAuthorJSON(a))
	}
	writeJSON(w, out)
}

func (m *Module) handleGetAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	a, err := m.store.GetAuthor(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	books, err := m.store.ListBooks(r.Context(), id)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	detail := authorDetailJSON{Author: toAuthorJSON(a), Books: make([]bookJSON, 0, len(books))}
	for _, b := range books {
		detail.Books = append(detail.Books, m.bookJSONWithFiles(r.Context(), b))
	}
	writeJSON(w, detail)
}

func (m *Module) handleListBooksHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListBooks(r.Context(), r.URL.Query().Get("author_id"))
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]bookJSON, 0, len(items))
	for _, b := range items {
		out = append(out, m.bookJSONWithFiles(r.Context(), b))
	}
	writeJSON(w, out)
}

func (m *Module) handleListMissingHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	items, total, err := m.store.ListMissingBooks(r.Context(), page, pageSize)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, missingBooksResponse{
		Items: items, Total: total, Page: page, PageSize: pageSize,
	})
}

func (m *Module) handleScanHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	res, err := m.ScanLibrary(r.Context())
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]int{
		"files_found": res.FilesFound, "files_imported": res.FilesImported,
		"files_skipped": res.FilesSkipped, "files_removed": res.FilesRemoved,
	})
}

func (m *Module) handleImportBookHTTP(w http.ResponseWriter, r *http.Request) {
	bookID := r.PathValue("id")
	if bookID == "" {
		http.Error(w, `{"error":"book id required"}`, http.StatusBadRequest)
		return
	}
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	f, err := m.store.ImportBookFile(r.Context(), bookID, body.Path, m.libraryRoot())
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, bookFileJSON{
		ID: f.ID, BookID: f.BookID, Title: f.Title, Path: f.Path,
		StreamURL: "/api/files/" + f.ID + "/stream",
	})
}

func (m *Module) handleStreamBookFileHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.NotFound(w, r)
		return
	}
	f, err := m.store.GetBookFile(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := validateLibraryPath(f.Path, m.libraryRoot())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

type authorJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GoodreadsID string `json:"goodreads_id"`
	Path        string `json:"path"`
	Monitored   bool   `json:"monitored"`
}

type bookFileJSON struct {
	ID        string `json:"id"`
	BookID    string `json:"book_id"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	StreamURL string `json:"stream_url,omitempty"`
}

type bookJSON struct {
	ID        string         `json:"id"`
	AuthorID  string         `json:"author_id"`
	Title     string         `json:"title"`
	ISBN      string         `json:"isbn"`
	Files     []bookFileJSON `json:"files,omitempty"`
	Year      int32          `json:"year"`
	Monitored bool           `json:"monitored"`
}

type authorDetailJSON struct {
	Author authorJSON `json:"author"`
	Books  []bookJSON `json:"books"`
}

type missingBooksResponse struct {
	Items    []MissingBook `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

func toAuthorJSON(a *Author) authorJSON {
	return authorJSON{
		ID: a.ID, Name: a.Name, GoodreadsID: a.GoodreadsID,
		Monitored: a.Monitored, Path: a.Path,
	}
}

func toBookJSON(b *Book) bookJSON {
	return bookJSON{
		ID: b.ID, AuthorID: b.AuthorID, Title: b.Title,
		ISBN: b.ISBN, Year: b.Year, Monitored: b.Monitored,
	}
}

func (m *Module) bookJSONWithFiles(ctx context.Context, b *Book) bookJSON {
	out := toBookJSON(b)
	if m.store == nil {
		return out
	}
	files, err := m.store.ListBookFiles(ctx, b.ID)
	if err != nil {
		return out
	}
	out.Files = make([]bookFileJSON, 0, len(files))
	for _, f := range files {
		streamURL := ""
		if _, err := validateLibraryPath(f.Path, m.libraryRoot()); err == nil {
			if _, err := os.Stat(f.Path); err == nil {
				streamURL = "/api/files/" + f.ID + "/stream"
			}
		}
		out.Files = append(out.Files, bookFileJSON{
			ID: f.ID, BookID: f.BookID, Title: f.Title, Path: f.Path,
			StreamURL: streamURL,
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func fmtJSONError(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}
