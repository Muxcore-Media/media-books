package internal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/core/sdk/go/module/pathguard"
)

func TestConfineRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.epub")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.epub")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	_, err := validateLibraryPath(link, root)
	if !errors.Is(err, pathguard.ErrOutsideRoots) {
		t.Fatalf("err=%v", err)
	}
	inside := filepath.Join(root, "ok.epub")
	if err := os.WriteFile(inside, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := validateLibraryPath(inside, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != inside {
		t.Fatalf("got %q", got)
	}
}
