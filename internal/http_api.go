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
	mux.HandleFunc("POST /api/authors", m.handleAddAuthorHTTP)
	mux.HandleFunc("GET /api/authors/{id}", m.handleGetAuthorHTTP)
	mux.HandleFunc("GET /api/authors/{id}/tags", m.handleGetAuthorTagsHTTP)
	mux.HandleFunc("PUT /api/authors/{id}/tags", m.handleSetAuthorTagsHTTP)
	mux.HandleFunc("POST /api/authors/{id}/books", m.handleAddBookHTTP)
	mux.HandleFunc("PATCH /api/authors/{id}", m.handlePatchAuthorHTTP)
	mux.HandleFunc("DELETE /api/authors/{id}", m.handleDeleteAuthorHTTP)
	mux.HandleFunc("GET /api/books", m.handleListBooksHTTP)
	mux.HandleFunc("PATCH /api/books/{id}", m.handlePatchBookHTTP)
	mux.HandleFunc("DELETE /api/books/{id}", m.handleDeleteBookHTTP)
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

func (m *Module) handleAddAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Monitored *bool  `json:"monitored"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}
	monitored := true
	if body.Monitored != nil {
		monitored = *body.Monitored
	}
	a, err := m.store.AddAuthor(r.Context(), Author{Name: name, Monitored: monitored})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, toAuthorJSON(a))
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

func (m *Module) handlePatchAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Monitored *bool   `json:"monitored"`
		Path      *string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	fields := map[string]any{}
	if body.Monitored != nil {
		fields["monitored"] = *body.Monitored
	}
	if body.Path != nil {
		fields["path"] = strings.TrimSpace(*body.Path)
	}
	if len(fields) == 0 {
		http.Error(w, `{"error":"monitored or path is required"}`, http.StatusBadRequest)
		return
	}
	a, err := m.store.UpdateAuthor(r.Context(), id, fields)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, toAuthorJSON(a))
}

func (m *Module) handleAddBookHTTP(w http.ResponseWriter, r *http.Request) {
	authorID := strings.TrimSpace(r.PathValue("id"))
	if authorID == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Monitored *bool  `json:"monitored"`
		Title     string `json:"title"`
		ISBN      string `json:"isbn"`
		Year      int32  `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	monitored := true
	if body.Monitored != nil {
		monitored = *body.Monitored
	}
	b, err := m.store.AddBook(r.Context(), Book{
		AuthorID: authorID, Title: title, ISBN: strings.TrimSpace(body.ISBN),
		Year: body.Year, Monitored: monitored,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, m.bookJSONWithFiles(r.Context(), b))
}

func (m *Module) handlePatchBookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	mon, ok := readMonitoredJSON(w, r)
	if !ok {
		return
	}
	b, err := m.store.UpdateBook(r.Context(), id, map[string]any{"monitored": mon})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, m.bookJSONWithFiles(r.Context(), b))
}

func (m *Module) handleDeleteAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	deleteFiles := queryDeleteFiles(r)
	if err := m.store.RemoveAuthorFiles(r.Context(), id, m.libraryRoot(), deleteFiles); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, map[string]any{"removed": true, "delete_files": deleteFiles})
}

func (m *Module) handleDeleteBookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	deleteFiles := queryDeleteFiles(r)
	if err := m.store.RemoveBookFiles(r.Context(), id, m.libraryRoot(), deleteFiles); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, map[string]any{"removed": true, "delete_files": deleteFiles})
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

func queryDeleteFiles(r *http.Request) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("delete_files"))
	return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
}

func readMonitoredJSON(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var body struct {
		Monitored *bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Monitored == nil {
		http.Error(w, `{"error":"monitored is required"}`, http.StatusBadRequest)
		return false, false
	}
	return *body.Monitored, true
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

func tagJSON(t *Tag) map[string]any {
	return map[string]any{"id": t.ID, "label": t.Label, "created_at": t.CreatedAt, "media": "book"}
}

func (m *Module) handleGetAuthorTagsHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if _, err := m.store.GetAuthor(r.Context(), id); err != nil {
		http.Error(w, fmtJSONError(err), http.StatusNotFound)
		return
	}
	tags, err := m.store.GetItemTags(r.Context(), id)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagJSON(t))
	}
	writeJSON(w, map[string]any{"available": true, "tags": out})
}

func (m *Module) handleSetAuthorTagsHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if _, err := m.store.GetAuthor(r.Context(), id); err != nil {
		http.Error(w, fmtJSONError(err), http.StatusNotFound)
		return
	}
	var body struct {
		TagIDs []string `json:"tag_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if err := m.store.SetItemTags(r.Context(), id, body.TagIDs); err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id, "tag_ids": body.TagIDs})
}
