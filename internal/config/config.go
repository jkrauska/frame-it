// Package config stores persistent frame-it settings (e.g. TV host).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File is the config filename inside the token directory.
const File = "config.json"

// Config holds saved frame-it settings.
type Config struct {
	Host             string `json:"host,omitempty"`
	LastWallpaperID  string `json:"last_wallpaper_id,omitempty"`
}

// Path returns the config file path for the given token directory.
func Path(tokenDir string) string {
	return filepath.Join(tokenDir, File)
}

// Load reads config from tokenDir/config.json. Missing file returns zero Config.
func Load(tokenDir string) (Config, error) {
	path := Path(tokenDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes cfg to tokenDir/config.json.
func Save(tokenDir string, cfg Config) error {
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	path := Path(tokenDir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// SetHost saves the TV host IP to config.
func SetHost(tokenDir, host string) error {
	cfg, err := Load(tokenDir)
	if err != nil {
		return err
	}
	cfg.Host = host
	return Save(tokenDir, cfg)
}

// SetLastWallpaperID records the most recent wallpaper upload content ID.
func SetLastWallpaperID(tokenDir, contentID string) error {
	cfg, err := Load(tokenDir)
	if err != nil {
		return err
	}
	cfg.LastWallpaperID = contentID
	return Save(tokenDir, cfg)
}
