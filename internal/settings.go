package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key: "library_dir", Label: "Library directory", Type: contracts.SettingTypeString,
			Value: m.libraryDir, Description: "Root path for book library (scanned offline)", Group: "Library",
		},
		{
			Key: "data_dir", Label: "Data directory", Type: contracts.SettingTypeString,
			Value: m.dataDir, Description: "Directory for SQLite library database (books.db)", Group: "Library",
		},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	switch key {
	case "library_dir":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("library_dir cannot be empty")
		}
		abs, err := filepath.Abs(filepath.Clean(value))
		if err != nil {
			return fmt.Errorf("library_dir: %w", err)
		}
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return fmt.Errorf("library_dir: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("library_dir: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("library_dir is not a directory")
		}
		m.libraryDir = abs
	case "data_dir":
		return fmt.Errorf("data_dir is set at startup (BOOKS_DATA_DIR); restart to change")
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}
