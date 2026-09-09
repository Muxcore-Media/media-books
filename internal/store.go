package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Author struct {
	ID          string
	Name        string
	GoodreadsID string
	Path        string
	Monitored   bool
}

type Book struct {
	ID        string
	AuthorID  string
	Title     string
	ISBN      string
	Year      int32
	Monitored bool
}

// BookFile is a single ebook file in the library.
type BookFile struct {
	ID       string
	BookID   string
	AuthorID string
	Title    string
	Path     string
}

// Store persists the book library in SQLite.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates the SQLite database at path (WAL mode).
func OpenStore(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS authors (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			goodreads_id  TEXT NOT NULL DEFAULT '',
			monitored     INTEGER NOT NULL DEFAULT 1,
			path          TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS books (
			id          TEXT PRIMARY KEY,
			author_id   TEXT NOT NULL,
			title       TEXT NOT NULL,
			isbn        TEXT NOT NULL DEFAULT '',
			year        INTEGER NOT NULL DEFAULT 0,
			monitored   INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (author_id) REFERENCES authors(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS book_files (
			id         TEXT PRIMARY KEY,
			book_id    TEXT NOT NULL,
			author_id  TEXT NOT NULL,
			title      TEXT NOT NULL,
			path       TEXT NOT NULL UNIQUE,
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
			FOREIGN KEY (author_id) REFERENCES authors(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(name);
		CREATE INDEX IF NOT EXISTS idx_books_author ON books(author_id);
		CREATE INDEX IF NOT EXISTS idx_book_files_book ON book_files(book_id);
		CREATE INDEX IF NOT EXISTS idx_book_files_path ON book_files(path);
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			source_title TEXT DEFAULT '',
			quality TEXT DEFAULT '',
			data TEXT DEFAULT '{}',
			date TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_history_item ON history(item_id);
	`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store not open")
	}
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one)
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) AddAuthor(ctx context.Context, a Author) (*Author, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, fmt.Errorf("author name required")
	}
	if a.ID == "" {
		a.ID = "au_" + uuid.NewString()[:8]
	}
	monitored := 0
	if a.Monitored {
		monitored = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO authors (id, name, goodreads_id, monitored, path)
		VALUES (?, ?, ?, ?, ?)
	`, a.ID, a.Name, a.GoodreadsID, monitored, a.Path)
	if err != nil {
		return nil, fmt.Errorf("insert author: %w", err)
	}
	out := a
	s.appendHistory(ctx, a.ID, historyImport, a.Name, "")
	return &out, nil
}

func (s *Store) GetAuthor(ctx context.Context, id string) (*Author, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, goodreads_id, monitored, path FROM authors WHERE id = ?
	`, id)
	return scanAuthor(row)
}

func (s *Store) ListAuthors(ctx context.Context, query string) ([]*Author, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, goodreads_id, monitored, path FROM authors ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list authors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]*Author, 0)
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) RemoveAuthor(ctx context.Context, id string) error {
	return s.RemoveAuthorFiles(ctx, id, "", false)
}

// RemoveAuthorFiles deletes an author and optionally unlinks ebook files under libraryRoot.
func (s *Store) RemoveAuthorFiles(ctx context.Context, id, libraryRoot string, deleteFiles bool) error {
	name := id
	if au, err := s.GetAuthor(ctx, id); err == nil && au != nil {
		name = au.Name
	}
	if deleteFiles && libraryRoot != "" {
		files, err := s.ListBookFiles(ctx, "")
		if err != nil {
			return err
		}
		for _, f := range files {
			if f.AuthorID != id {
				continue
			}
			if err := unlinkIfUnderRoot(f.Path, libraryRoot); err != nil {
				return err
			}
		}
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM authors WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete author: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("author %q not found", id)
	}
	s.appendHistory(ctx, id, historyDeleteItem, name, "")
	return nil
}

func (s *Store) AddBook(ctx context.Context, b Book) (*Book, error) {
	if strings.TrimSpace(b.Title) == "" {
		return nil, fmt.Errorf("book title required")
	}
	var exists string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM authors WHERE id = ?`, b.AuthorID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("author %q not found", b.AuthorID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup author: %w", err)
	}
	if b.ID == "" {
		b.ID = "bk_" + uuid.NewString()[:8]
	}
	monitored := 0
	if b.Monitored {
		monitored = 1
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO books (id, author_id, title, isbn, year, monitored)
		VALUES (?, ?, ?, ?, ?, ?)
	`, b.ID, b.AuthorID, b.Title, b.ISBN, b.Year, monitored)
	if err != nil {
		return nil, fmt.Errorf("insert book: %w", err)
	}
	out := b
	return &out, nil
}

func (s *Store) GetBook(ctx context.Context, id string) (*Book, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, author_id, title, isbn, year, monitored FROM books WHERE id = ?
	`, id)
	return scanBook(row)
}

func (s *Store) UpdateAuthor(ctx context.Context, id string, fields map[string]any) (*Author, error) {
	a, err := s.GetAuthor(ctx, id)
	if err != nil {
		return nil, err
	}
	if v, ok := fields["name"].(string); ok && strings.TrimSpace(v) != "" {
		a.Name = v
	}
	if v, ok := fields["goodreads_id"].(string); ok {
		a.GoodreadsID = v
	}
	if v, ok := fields["path"].(string); ok {
		a.Path = v
	}
	if v, ok := fields["monitored"].(bool); ok {
		a.Monitored = v
	}
	monitored := 0
	if a.Monitored {
		monitored = 1
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE authors SET name = ?, goodreads_id = ?, monitored = ?, path = ? WHERE id = ?
	`, a.Name, a.GoodreadsID, monitored, a.Path, id)
	if err != nil {
		return nil, fmt.Errorf("update author: %w", err)
	}
	return s.GetAuthor(ctx, id)
}

func (s *Store) UpdateBook(ctx context.Context, id string, fields map[string]any) (*Book, error) {
	b, err := s.GetBook(ctx, id)
	if err != nil {
		return nil, err
	}
	if v, ok := fields["title"].(string); ok && strings.TrimSpace(v) != "" {
		b.Title = v
	}
	if v, ok := fields["isbn"].(string); ok {
		b.ISBN = v
	}
	if v, ok := fields["year"].(int32); ok {
		b.Year = v
	}
	if v, ok := fields["monitored"].(bool); ok {
		b.Monitored = v
	}
	monitored := 0
	if b.Monitored {
		monitored = 1
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE books SET title = ?, isbn = ?, year = ?, monitored = ? WHERE id = ?
	`, b.Title, b.ISBN, b.Year, monitored, id)
	if err != nil {
		return nil, fmt.Errorf("update book: %w", err)
	}
	return s.GetBook(ctx, id)
}

func (s *Store) RemoveBook(ctx context.Context, id string) error {
	return s.RemoveBookFiles(ctx, id, "", false)
}

// RemoveBookFiles deletes a book and optionally unlinks ebook files under libraryRoot.
func (s *Store) RemoveBookFiles(ctx context.Context, id, libraryRoot string, deleteFiles bool) error {
	title, authorID := id, ""
	if bk, err := s.GetBook(ctx, id); err == nil && bk != nil {
		title, authorID = bk.Title, bk.AuthorID
	}
	if deleteFiles && libraryRoot != "" {
		files, err := s.ListBookFiles(ctx, id)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := unlinkIfUnderRoot(f.Path, libraryRoot); err != nil {
				return err
			}
		}
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete book: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("book %q not found", id)
	}
	if authorID != "" {
		s.appendHistory(ctx, authorID, historyDeleteFile, title, "")
	}
	return nil
}

func (s *Store) GetBookFile(ctx context.Context, id string) (*BookFile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, book_id, author_id, title, path FROM book_files WHERE id = ?
	`, id)
	return scanBookFile(row)
}

// ImportBookFile attaches an on-disk file under libraryRoot to a book.
func (s *Store) ImportBookFile(ctx context.Context, bookID, filePath, libraryRoot string) (*BookFile, error) {
	abs, err := validateLibraryPath(filePath, libraryRoot)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	bk, err := s.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	au, err := s.GetAuthor(ctx, bk.AuthorID)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)))
	f, err := s.upsertBookFile(ctx, BookFile{
		BookID:   bk.ID,
		AuthorID: au.ID,
		Title:    title,
		Path:     abs,
	})
	if err != nil {
		return nil, err
	}
	s.appendHistory(ctx, au.ID, historyImport, title, "")
	return f, nil
}

func unlinkIfUnderRoot(path, root string) error {
	abs, err := pathUnderRoot(path, root)
	if err != nil {
		return nil
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unlink %q: %w", abs, err)
	}
	return nil
}

func (s *Store) bookHasOnDiskFile(ctx context.Context, bookID string) (bool, error) {
	files, err := s.ListBookFiles(ctx, bookID)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		if _, err := os.Stat(f.Path); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) ListBooks(ctx context.Context, authorID string) ([]*Book, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if authorID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, author_id, title, isbn, year, monitored
			FROM books WHERE author_id = ? ORDER BY year, title
		`, authorID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, author_id, title, isbn, year, monitored
			FROM books ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*Book, 0)
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListBookFiles returns book files, optionally filtered by book ID.
func (s *Store) ListBookFiles(ctx context.Context, bookID string) ([]*BookFile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if bookID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, book_id, author_id, title, path FROM book_files
			WHERE book_id = ? ORDER BY title
		`, bookID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, book_id, author_id, title, path FROM book_files ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list book files: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*BookFile, 0)
	for rows.Next() {
		f, err := scanBookFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MissingBook is a monitored book with no files on disk.
type MissingBook struct {
	BookID     string `json:"book_id"`
	AuthorID   string `json:"author_id"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	Year       int32  `json:"year"`
}

// ListMissingBooks returns monitored books with no on-disk ebook files.
func (s *Store) ListMissingBooks(ctx context.Context, page, pageSize int) ([]MissingBook, int, error) {
	return s.ListMissingBooksFiltered(ctx, page, pageSize, "")
}

// ListMissingBooksFiltered returns monitored missing books, optionally scoped to one author.
func (s *Store) ListMissingBooksFiltered(ctx context.Context, page, pageSize int, authorID string) ([]MissingBook, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 100
	}
	q := `
		SELECT b.id, b.author_id, b.title, b.year, a.name
		FROM books b
		JOIN authors a ON a.id = b.author_id
		WHERE b.monitored = 1 AND a.monitored = 1`
	args := []any{}
	if authorID != "" {
		q += ` AND b.author_id = ?`
		args = append(args, authorID)
	}
	q += ` ORDER BY a.name, b.title`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list candidate missing books: %w", err)
	}
	candidates := make([]MissingBook, 0)
	for rows.Next() {
		var item MissingBook
		if err := rows.Scan(&item.BookID, &item.AuthorID, &item.Title, &item.Year, &item.AuthorName); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	all := make([]MissingBook, 0, len(candidates))
	for _, item := range candidates {
		hasFile, err := s.bookHasOnDiskFile(ctx, item.BookID)
		if err != nil {
			return nil, 0, err
		}
		if !hasFile {
			all = append(all, item)
		}
	}
	total := len(all)
	offset := (page - 1) * pageSize
	if offset >= total {
		return []MissingBook{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// PurgeMissingBookFiles deletes book_files rows whose paths no longer exist on disk.
func (s *Store) PurgeMissingBookFiles(ctx context.Context) (int, error) {
	files, err := s.ListBookFiles(ctx, "")
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range files {
		if _, err := os.Stat(f.Path); err == nil {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM book_files WHERE id = ?`, f.ID); err != nil {
			return removed, fmt.Errorf("purge book file: %w", err)
		}
		removed++
	}
	return removed, nil
}

func (s *Store) findAuthorByName(ctx context.Context, name string) (*Author, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, goodreads_id, monitored, path FROM authors
		WHERE lower(name) = lower(?) LIMIT 1
	`, name)
	a, err := scanAuthor(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (s *Store) findBook(ctx context.Context, authorID, title string) (*Book, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, author_id, title, isbn, year, monitored FROM books
		WHERE author_id = ? AND lower(title) = lower(?) LIMIT 1
	`, authorID, title)
	b, err := scanBook(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (s *Store) findBookFileByPath(ctx context.Context, path string) (*BookFile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, book_id, author_id, title, path FROM book_files WHERE path = ?
	`, path)
	f, err := scanBookFile(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

func (s *Store) upsertBookFile(ctx context.Context, f BookFile) (*BookFile, error) {
	existing, err := s.findBookFileByPath(ctx, f.Path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		_, err = s.db.ExecContext(ctx, `
			UPDATE book_files SET book_id = ?, author_id = ?, title = ? WHERE id = ?
		`, f.BookID, f.AuthorID, f.Title, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("update book file: %w", err)
		}
		existing.BookID = f.BookID
		existing.AuthorID = f.AuthorID
		existing.Title = f.Title
		return existing, nil
	}
	if f.ID == "" {
		f.ID = "bf_" + uuid.NewString()[:8]
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, author_id, title, path)
		VALUES (?, ?, ?, ?, ?)
	`, f.ID, f.BookID, f.AuthorID, f.Title, f.Path)
	if err != nil {
		return nil, fmt.Errorf("insert book file: %w", err)
	}
	out := f
	return &out, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAuthor(row rowScanner) (*Author, error) {
	var a Author
	var monitored int
	if err := row.Scan(&a.ID, &a.Name, &a.GoodreadsID, &monitored, &a.Path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("author not found")
		}
		return nil, err
	}
	a.Monitored = monitored != 0
	return &a, nil
}

func scanBook(row rowScanner) (*Book, error) {
	var b Book
	var monitored int
	if err := row.Scan(&b.ID, &b.AuthorID, &b.Title, &b.ISBN, &b.Year, &monitored); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("book not found")
		}
		return nil, err
	}
	b.Monitored = monitored != 0
	return &b, nil
}

func scanBookFile(row rowScanner) (*BookFile, error) {
	var f BookFile
	if err := row.Scan(&f.ID, &f.BookID, &f.AuthorID, &f.Title, &f.Path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("book file not found")
		}
		return nil, err
	}
	return &f, nil
}
