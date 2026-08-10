package internal

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
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

type Store struct {
	mu      sync.RWMutex
	authors map[string]*Author
	books   map[string]*Book
}

func NewStore() *Store {
	return &Store{authors: map[string]*Author{}, books: map[string]*Book{}}
}

func (s *Store) AddAuthor(a Author) (*Author, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, fmt.Errorf("author name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == "" {
		a.ID = "au_" + uuid.NewString()[:8]
	}
	cp := a
	s.authors[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetAuthor(id string) (*Author, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.authors[id]
	if !ok {
		return nil, fmt.Errorf("author %q not found", id)
	}
	cp := *a
	return &cp, nil
}

func (s *Store) ListAuthors(query string) []*Author {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]*Author, 0, len(s.authors))
	for _, a := range s.authors {
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out
}

func (s *Store) RemoveAuthor(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.authors[id]; !ok {
		return fmt.Errorf("author %q not found", id)
	}
	delete(s.authors, id)
	for bid, b := range s.books {
		if b.AuthorID == id {
			delete(s.books, bid)
		}
	}
	return nil
}

func (s *Store) AddBook(b Book) (*Book, error) {
	if strings.TrimSpace(b.Title) == "" {
		return nil, fmt.Errorf("book title required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.authors[b.AuthorID]; !ok {
		return nil, fmt.Errorf("author %q not found", b.AuthorID)
	}
	if b.ID == "" {
		b.ID = "bk_" + uuid.NewString()[:8]
	}
	cp := b
	s.books[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) ListBooks(authorID string) []*Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Book, 0)
	for _, b := range s.books {
		if authorID != "" && b.AuthorID != authorID {
			continue
		}
		cp := *b
		out = append(out, &cp)
	}
	return out
}
