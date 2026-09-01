package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func pathUnderRoot(path, root string) (string, error) {
	path = strings.TrimSpace(path)
	root = strings.TrimSpace(root)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if root == "" {
		return "", fmt.Errorf("library root is not configured")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	abs = filepath.Clean(abs)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve library root: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", fmt.Errorf("path outside library root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside library root")
	}
	return abs, nil
}

func validateLibraryPath(path, root string) (string, error) {
	abs, err := pathUnderRoot(path, root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return abs, nil
		}
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	if _, err := pathUnderRoot(resolved, root); err != nil {
		return "", fmt.Errorf("symlink target outside library root")
	}
	return resolved, nil
}
