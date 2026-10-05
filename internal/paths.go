package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Muxcore-Media/core/sdk/go/module/pathguard"
)

func pathUnderRoot(path, root string) (string, error) {
	return confineToRoot(path, root)
}

func validateLibraryPath(path, root string) (string, error) {
	return confineToRoot(path, root)
}

// confineToRoot resolves path inside root with pathguard. Symlinks on the
// nearest existing ancestor are followed, and a sibling prefix such as
// /library2 is not inside /library.
func confineToRoot(path, root string) (string, error) {
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
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve library root: %w", err)
	}
	resolved, err := pathguard.Confine(abs, []string{rootAbs})
	if err != nil {
		return "", fmt.Errorf("path outside library root: %w", err)
	}
	return resolved, nil
}
