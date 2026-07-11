package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func LoadFromPaths(path, legacyPath string) Config {
	if cfg, ok := Read(path); ok && len(cfg.Groups) > 0 {
		return cfg
	}
	if cfg, ok := Read(legacyPath); ok {
		if err := WriteFile(path, cfg); err == nil {
			_ = os.Remove(legacyPath)
		}
		if len(cfg.Groups) > 0 {
			return cfg
		}
	}
	return Default()
}

func Read(path string) (Config, bool) {
	cfg := Base()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return NormalizeLoaded(cfg), true
}

func WriteFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
