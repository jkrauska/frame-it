// Package config stores persistent frame-it settings (e.g. TV host).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// File is the config filename inside the token directory.
const File = "config.json"

// Config holds saved frame-it settings.
type Config struct {
	Host              string `json:"host,omitempty"`
	LastWallpaperID   string `json:"last_wallpaper_id,omitempty"` // active slot; kept for compatibility
	WallpaperSlot1    string `json:"wallpaper_slot1,omitempty"`
	WallpaperSlot2    string `json:"wallpaper_slot2,omitempty"`
	WallpaperActive   int    `json:"wallpaper_active_slot,omitempty"` // 1 or 2

	// API keys for wallpaper sources. Used when no flag/env value is set.
	WallhavenKey string `json:"wallhaven_key,omitempty"`
	UnsplashKey  string `json:"unsplash_key,omitempty"`
	PixabayKey   string `json:"pixabay_key,omitempty"`

	// ImagesDir overrides where pure downloaded images are archived.
	// KeepImages overrides how many archived images to retain (0 disables archiving).
	ImagesDir  string `json:"images_dir,omitempty"`
	KeepImages *int   `json:"keep_images,omitempty"`
}

// Wallpaper slot labels stored in config (logical names; TV assigns its own content IDs).
const (
	WallpaperSlot1Name = "frame-it-image1"
	WallpaperSlot2Name = "frame-it-image2"
)

// WallpaperSlotLabel returns the logical slot name.
func WallpaperSlotLabel(slot int) string {
	switch slot {
	case 1:
		return WallpaperSlot1Name
	case 2:
		return WallpaperSlot2Name
	default:
		return fmt.Sprintf("frame-it-image%d", slot)
	}
}

// WallpaperSlots is the double-buffer state used for wallpaper replacement.
type WallpaperSlots struct {
	Slot1  string
	Slot2  string
	Active int // 1, 2, or 0 when unset
}

// WallpaperSlots returns slot state, migrating legacy last_wallpaper_id when needed.
func (c Config) WallpaperSlots() WallpaperSlots {
	slots := WallpaperSlots{
		Slot1:  c.WallpaperSlot1,
		Slot2:  c.WallpaperSlot2,
		Active: c.WallpaperActive,
	}
	if slots.Active == 0 && c.LastWallpaperID != "" {
		slots.Slot1 = c.LastWallpaperID
		slots.Active = 1
	}
	return slots
}

// NextWallpaperTarget picks the slot to write next and any prior content ID to remove after the swap.
func NextWallpaperTarget(slots WallpaperSlots) (targetSlot int, replaceID string) {
	switch slots.Active {
	case 1:
		return 2, slots.Slot2
	case 2:
		return 1, slots.Slot1
	default:
		return 1, ""
	}
}

// LoadWallpaperSlots reads slot state and persists legacy last_wallpaper_id into slot 1 when needed.
func LoadWallpaperSlots(tokenDir string) (WallpaperSlots, error) {
	cfg, err := Load(tokenDir)
	if err != nil {
		return WallpaperSlots{}, err
	}
	if cfg.WallpaperActive == 0 && cfg.LastWallpaperID != "" && cfg.WallpaperSlot1 == "" {
		cfg.WallpaperSlot1 = cfg.LastWallpaperID
		cfg.WallpaperActive = 1
		if err := Save(tokenDir, cfg); err != nil {
			return WallpaperSlots{}, err
		}
	}
	return cfg.WallpaperSlots(), nil
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

// SettableKeys are the keys accepted by Set/Get, in display order.
var SettableKeys = []string{
	"host",
	"wallhaven-key",
	"unsplash-key",
	"pixabay-key",
	"images-dir",
	"keep-images",
}

// Set updates a single config field by key and saves it. An empty value clears
// the field. The keep-images value must parse as a non-negative integer.
func Set(tokenDir, key, value string) error {
	cfg, err := Load(tokenDir)
	if err != nil {
		return err
	}
	switch key {
	case "host":
		cfg.Host = value
	case "wallhaven-key":
		cfg.WallhavenKey = value
	case "unsplash-key":
		cfg.UnsplashKey = value
	case "pixabay-key":
		cfg.PixabayKey = value
	case "images-dir":
		cfg.ImagesDir = value
	case "keep-images":
		if value == "" {
			cfg.KeepImages = nil
			break
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("keep-images must be a non-negative integer, got %q", value)
		}
		cfg.KeepImages = &n
	default:
		return fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(SettableKeys, ", "))
	}
	return Save(tokenDir, cfg)
}

// Get returns the string form of a single config field by key.
func Get(tokenDir, key string) (string, error) {
	cfg, err := Load(tokenDir)
	if err != nil {
		return "", err
	}
	switch key {
	case "host":
		return cfg.Host, nil
	case "wallhaven-key":
		return cfg.WallhavenKey, nil
	case "unsplash-key":
		return cfg.UnsplashKey, nil
	case "pixabay-key":
		return cfg.PixabayKey, nil
	case "images-dir":
		return cfg.ImagesDir, nil
	case "keep-images":
		if cfg.KeepImages == nil {
			return "", nil
		}
		return strconv.Itoa(*cfg.KeepImages), nil
	default:
		return "", fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(SettableKeys, ", "))
	}
}

// Display returns key/value pairs for all settable keys, masking secret values.
func (c Config) Display() []KeyValue {
	mask := func(s string) string {
		if s == "" {
			return ""
		}
		if len(s) <= 4 {
			return "****"
		}
		return "****" + s[len(s)-4:]
	}
	keep := ""
	if c.KeepImages != nil {
		keep = strconv.Itoa(*c.KeepImages)
	}
	return []KeyValue{
		{"host", c.Host},
		{"wallhaven-key", mask(c.WallhavenKey)},
		{"unsplash-key", mask(c.UnsplashKey)},
		{"pixabay-key", mask(c.PixabayKey)},
		{"images-dir", c.ImagesDir},
		{"keep-images", keep},
	}
}

// KeyValue is a single displayed config entry.
type KeyValue struct {
	Key   string
	Value string
}

// SetWallpaperSlot saves a slot's content ID and marks it active.
func SetWallpaperSlot(tokenDir string, slot int, contentID string) error {
	cfg, err := Load(tokenDir)
	if err != nil {
		return err
	}
	switch slot {
	case 1:
		cfg.WallpaperSlot1 = contentID
	case 2:
		cfg.WallpaperSlot2 = contentID
	default:
		return fmt.Errorf("invalid wallpaper slot %d", slot)
	}
	cfg.WallpaperActive = slot
	cfg.LastWallpaperID = contentID
	return Save(tokenDir, cfg)
}
