package history

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func CreateAutosaveFile(dir string, startedAt time.Time, pid int) (string, error) {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	if pid <= 0 {
		pid = os.Getpid()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	stem := "history-" + startedAt.Format("2006-01-02_15.04.05")
	for suffix := 0; ; suffix++ {
		path := filepath.Join(dir, AutosaveFilename(stem, pid, suffix))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			closeErr := file.Close()
			if closeErr != nil {
				return "", closeErr
			}
			return path, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
}

func AutosaveFilename(stem string, pid, suffix int) string {
	switch suffix {
	case 0:
		return stem + ".json"
	case 1:
		return fmt.Sprintf("%s-pid%d.json", stem, pid)
	default:
		return fmt.Sprintf("%s-pid%d-%d.json", stem, pid, suffix)
	}
}
