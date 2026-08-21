package internal

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (m *Module) registerBooksHTTPAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", m.handleListAuthorsHTTP)
	mux.HandleFunc("GET /api/authors/{id}", m.handleGetAuthorHTTP)
	mux.HandleFunc("GET /api/books", m.handleListBooksHTTP)
	mux.HandleFunc("GET /api/files/{id}/stream", m.handleStreamBookFileHTTP)
}

func (m *Module) handleListAuthorsHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListAuthors(r.URL.Query().Get("q"))
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
	a, err := m.store.GetAuthor(id)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	books, err := m.store.ListBooks(id)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	detail := authorDetailJSON{Author: toAuthorJSON(a), Books: make([]bookJSON, 0, len(books))}
	for _, b := range books {
		detail.Books = append(detail.Books, m.bookJSONWithFiles(b))
	}
	writeJSON(w, detail)
}

func (m *Module) handleListBooksHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListBooks(r.URL.Query().Get("author_id"))
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]bookJSON, 0, len(items))
	for _, b := range items {
		out = append(out, m.bookJSONWithFiles(b))
	}
	writeJSON(w, out)
}

func (m *Module) handleStreamBookFileHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.NotFound(w, r)
		return
	}
	files, err := m.store.ListBookFiles("")
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	var path string
	for _, f := range files {
		if f.ID == id {
			path = f.Path
			break
		}
	}
	if path == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

type authorJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GoodreadsID string `json:"goodreads_id"`
	Monitored   bool   `json:"monitored"`
	Path        string `json:"path"`
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
	Year      int32          `json:"year"`
	Monitored bool           `json:"monitored"`
	Files     []bookFileJSON `json:"files,omitempty"`
}

type authorDetailJSON struct {
	Author authorJSON `json:"author"`
	Books  []bookJSON `json:"books"`
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

func (m *Module) bookJSONWithFiles(b *Book) bookJSON {
	out := toBookJSON(b)
	if m.store == nil {
		return out
	}
	files, err := m.store.ListBookFiles(b.ID)
	if err != nil {
		return out
	}
	out.Files = make([]bookFileJSON, 0, len(files))
	for _, f := range files {
		out.Files = append(out.Files, bookFileJSON{
			ID: f.ID, BookID: f.BookID, Title: f.Title, Path: f.Path,
			StreamURL: "/stream/books/" + f.ID,
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
