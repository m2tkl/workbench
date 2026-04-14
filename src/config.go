package workbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DataDir string `json:"data_dir,omitempty"`
}

func defaultConfigDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("TASKBENCH_CONFIG_DIR")); dir != "" {
		return filepath.Abs(dir)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workbench"), nil
}

func defaultConfigPath() (string, error) {
	dir, err := defaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func loadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return normalizeConfig(config)
}

func saveConfig(path string, config Config) error {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func normalizeConfig(config Config) (Config, error) {
	config.DataDir = strings.TrimSpace(config.DataDir)
	if config.DataDir == "" {
		return config, nil
	}

	abs, err := filepath.Abs(config.DataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data_dir: %w", err)
	}
	config.DataDir = abs
	return config, nil
}
