package history

import (
	"errors"
	"os"
	"path/filepath"
)

func MigrateLegacyFile(path, legacyPath string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.Rename(legacyPath, path); err == nil {
		return nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return os.Remove(legacyPath)
}
