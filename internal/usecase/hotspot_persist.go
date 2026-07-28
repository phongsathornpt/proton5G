package usecase

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"fm350-monitor/internal/pkg/domain"
)

func loadHotspotConfigFile(path string) (domain.HotspotConfig, error) {
	var cfg domain.HotspotConfig
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse hotspot config: %w", err)
	}
	return cfg, nil
}

func saveHotspotConfigFile(path string, cfg domain.HotspotConfig) error {
	if path == "" {
		return fmt.Errorf("hotspot config path empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
