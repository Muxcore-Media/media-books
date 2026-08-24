package internal

import (
	"database/sql"
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
	Monitored   bool
	Path        string
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
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
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
	`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) AddAuthor(a Author) (*Author, error) {
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
	_, err := s.db.Exec(`
		INSERT INTO authors (id, name, goodreads_id, monitored, path)
		VALUES (?, ?, ?, ?, ?)
	`, a.ID, a.Name, a.GoodreadsID, monitored, a.Path)
	if err != nil {
		return nil, fmt.Errorf("insert author: %w", err)
	}
	out := a
	return &out, nil
}

func (s *Store) GetAuthor(id string) (*Author, error) {
	row := s.db.QueryRow(`
		SELECT id, name, goodreads_id, monitored, path FROM authors WHERE id = ?
	`, id)
	return scanAuthor(row)
}

func (s *Store) ListAuthors(query string) ([]*Author, error) {
	rows, err := s.db.Query(`
		SELECT id, name, goodreads_id, monitored, path FROM authors ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list authors: %w", err)
	}
	defer rows.Close()
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

func (s *Store) RemoveAuthor(id string) error {
	res, err := s.db.Exec(`DELETE FROM authors WHERE id = ?`, id)
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
	// Cascades may be off without PRAGMA foreign_keys; clean children explicitly.
	_, _ = s.db.Exec(`DELETE FROM book_files WHERE author_id = ?`, id)
	_, _ = s.db.Exec(`DELETE FROM books WHERE author_id = ?`, id)
	return nil
}

func (s *Store) AddBook(b Book) (*Book, error) {
	if strings.TrimSpace(b.Title) == "" {
		return nil, fmt.Errorf("book title required")
	}
	var exists string
	err := s.db.QueryRow(`SELECT id FROM authors WHERE id = ?`, b.AuthorID).Scan(&exists)
	if err == sql.ErrNoRows {
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
	_, err = s.db.Exec(`
		INSERT INTO books (id, author_id, title, isbn, year, monitored)
		VALUES (?, ?, ?, ?, ?, ?)
	`, b.ID, b.AuthorID, b.Title, b.ISBN, b.Year, monitored)
	if err != nil {
		return nil, fmt.Errorf("insert book: %w", err)
	}
	out := b
	return &out, nil
}

func (s *Store) ListBooks(authorID string) ([]*Book, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if authorID != "" {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, isbn, year, monitored
			FROM books WHERE author_id = ? ORDER BY year, title
		`, authorID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, isbn, year, monitored
			FROM books ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()
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
func (s *Store) ListBookFiles(bookID string) ([]*BookFile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if bookID != "" {
		rows, err = s.db.Query(`
			SELECT id, book_id, author_id, title, path FROM book_files
			WHERE book_id = ? ORDER BY title
		`, bookID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, book_id, author_id, title, path FROM book_files ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list book files: %w", err)
	}
	defer rows.Close()
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
	BookID     string
	AuthorID   string
	Title      string
	AuthorName string
	Year       int32
}

// ListMissingBooks returns monitored books that have no book_files rows.
func (s *Store) ListMissingBooks(page, pageSize int) ([]MissingBook, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	var total int
	if err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM books b
		JOIN authors a ON a.id = b.author_id
		WHERE b.monitored = 1 AND a.monitored = 1
		  AND NOT EXISTS (SELECT 1 FROM book_files f WHERE f.book_id = b.id)
	`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count missing books: %w", err)
	}
	rows, err := s.db.Query(`
		SELECT b.id, b.author_id, b.title, b.year, a.name
		FROM books b
		JOIN authors a ON a.id = b.author_id
		WHERE b.monitored = 1 AND a.monitored = 1
		  AND NOT EXISTS (SELECT 1 FROM book_files f WHERE f.book_id = b.id)
		ORDER BY a.name, b.title
		LIMIT ? OFFSET ?
	`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list missing books: %w", err)
	}
	defer rows.Close()
	out := make([]MissingBook, 0)
	for rows.Next() {
		var item MissingBook
		if err := rows.Scan(&item.BookID, &item.AuthorID, &item.Title, &item.Year, &item.AuthorName); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) findAuthorByName(name string) (*Author, error) {
	row := s.db.QueryRow(`
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

func (s *Store) findBook(authorID, title string) (*Book, error) {
	row := s.db.QueryRow(`
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

func (s *Store) findBookFileByPath(path string) (*BookFile, error) {
	row := s.db.QueryRow(`
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

func (s *Store) upsertBookFile(f BookFile) (*BookFile, error) {
	existing, err := s.findBookFileByPath(f.Path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		_, err := s.db.Exec(`
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
	_, err = s.db.Exec(`
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
		if err == sql.ErrNoRows {
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
		if err == sql.ErrNoRows {
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
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("book file not found")
		}
		return nil, err
	}
	return &f, nil
}
