package internal

import "context"

// LibraryDir returns the configured library root (tests).
func (m *Module) LibraryDir() string { return m.libraryRoot() }

// Store exposes the SQLite store for tests.
func (m *Module) Store() *Store { return m.store }

// UpsertBookFileForTest inserts or updates a book_files row (tests only).
func (s *Store) UpsertBookFileForTest(ctx context.Context, f BookFile) (*BookFile, error) {
	return s.upsertBookFile(ctx, f)
}
