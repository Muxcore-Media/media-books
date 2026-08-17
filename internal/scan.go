package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ebook extensions recognized during library scans (local files only; no metadata APIs).
var ebookExts = map[string]struct{}{
	".epub": {},
	".mobi": {},
	".azw":  {},
	".azw3": {},
	".pdf":  {},
	".fb2":  {},
	".txt":  {},
}

// ScanResult summarizes a library root scan.
type ScanResult struct {
	FilesFound    int
	FilesImported int
	FilesSkipped  int
}

// ScanLibraryRoot walks root for ebook files and upserts authors/books/files.
// Layout expected: Author/Book Title/file.ext (Author/file.ext → title from filename).
// Metadata is derived only from path/filename — no network lookups.
func (s *Store) ScanLibraryRoot(root string) (*ScanResult, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("library root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("library root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("library root is not a directory: %s", root)
	}

	res := &ScanResult{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := ebookExts[ext]; !ok {
			return nil
		}
		res.FilesFound++
		abs, err := filepath.Abs(path)
		if err != nil {
			res.FilesSkipped++
			return nil
		}
		authorName, bookTitle, fileTitle := inferFromPath(root, abs)
		imported, err := s.importEbookFile(authorName, bookTitle, fileTitle, abs)
		if err != nil {
			return err
		}
		if imported {
			res.FilesImported++
		} else {
			res.FilesSkipped++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Store) importEbookFile(authorName, bookTitle, fileTitle, absPath string) (imported bool, err error) {
	existing, err := s.findBookFileByPath(absPath)
	if err != nil {
		return false, err
	}

	au, err := s.findAuthorByName(authorName)
	if err != nil {
		return false, err
	}
	if au == nil {
		au, err = s.AddAuthor(Author{
			Name:      authorName,
			Monitored: true,
			Path:      filepath.Dir(filepath.Dir(absPath)),
		})
		if err != nil {
			return false, err
		}
	}

	bk, err := s.findBook(au.ID, bookTitle)
	if err != nil {
		return false, err
	}
	if bk == nil {
		bk, err = s.AddBook(Book{
			AuthorID:  au.ID,
			Title:     bookTitle,
			Monitored: true,
		})
		if err != nil {
			return false, err
		}
	}

	_, err = s.upsertBookFile(BookFile{
		BookID:   bk.ID,
		AuthorID: au.ID,
		Title:    fileTitle,
		Path:     absPath,
	})
	if err != nil {
		return false, err
	}
	return existing == nil, nil
}

// inferFromPath derives author/book/file title from Author/Book/file.ext under root.
func inferFromPath(root, absPath string) (author, book, fileTitle string) {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		rel = filepath.Base(absPath)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	base := parts[len(parts)-1]
	fileTitle = strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	switch len(parts) {
	case 1:
		return "Unknown Author", fileTitle, fileTitle
	case 2:
		return parts[0], fileTitle, fileTitle
	default:
		return parts[0], parts[1], fileTitle
	}
}
