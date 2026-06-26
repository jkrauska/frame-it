package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()

	if err := SetHost(dir, "192.168.7.133"); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "192.168.7.133" {
		t.Fatalf("host = %q, want 192.168.7.133", cfg.Host)
	}

	data, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `{
  "host": "192.168.7.133"
}
`
	if string(data) != want {
		t.Fatalf("config file:\n%s\nwant:\n%s", data, want)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "" {
		t.Fatalf("host = %q, want empty", cfg.Host)
	}
}

func TestNextWallpaperTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		slots      WallpaperSlots
		wantTarget int
		wantReplace string
	}{
		{
			name:       "first upload",
			slots:      WallpaperSlots{},
			wantTarget: 1,
		},
		{
			name:        "active slot 1 reuses slot 2",
			slots:       WallpaperSlots{Slot1: "MY_A", Slot2: "MY_B", Active: 1},
			wantTarget:  2,
			wantReplace: "MY_B",
		},
		{
			name:        "active slot 2 reuses slot 1",
			slots:       WallpaperSlots{Slot1: "MY_A", Slot2: "MY_B", Active: 2},
			wantTarget:  1,
			wantReplace: "MY_A",
		},
		{
			name:       "active slot 1 empty inactive slot",
			slots:      WallpaperSlots{Slot1: "MY_A", Active: 1},
			wantTarget: 2,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			target, replaceID := NextWallpaperTarget(tc.slots)
			if target != tc.wantTarget {
				t.Fatalf("target = %d, want %d", target, tc.wantTarget)
			}
			if replaceID != tc.wantReplace {
				t.Fatalf("replaceID = %q, want %q", replaceID, tc.wantReplace)
			}
		})
	}
}

func TestWallpaperSlotLabel(t *testing.T) {
	if WallpaperSlotLabel(1) != WallpaperSlot1Name {
		t.Fatalf("slot 1 label = %q", WallpaperSlotLabel(1))
	}
	if WallpaperSlotLabel(2) != WallpaperSlot2Name {
		t.Fatalf("slot 2 label = %q", WallpaperSlotLabel(2))
	}
}

func TestLoadWallpaperSlotsMigratesLegacy(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Config{LastWallpaperID: "MY_OLD"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	slots, err := LoadWallpaperSlots(dir)
	if err != nil {
		t.Fatalf("LoadWallpaperSlots: %v", err)
	}
	if slots.Slot1 != "MY_OLD" || slots.Active != 1 {
		t.Fatalf("slots = %+v, want slot1=MY_OLD active=1", slots)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WallpaperSlot1 != "MY_OLD" || cfg.WallpaperActive != 1 {
		t.Fatalf("persisted cfg = %+v", cfg)
	}
}

func TestSetWallpaperSlot(t *testing.T) {
	dir := t.TempDir()

	if err := SetWallpaperSlot(dir, 2, "MY_NEW"); err != nil {
		t.Fatalf("SetWallpaperSlot: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WallpaperSlot2 != "MY_NEW" || cfg.WallpaperActive != 2 || cfg.LastWallpaperID != "MY_NEW" {
		t.Fatalf("cfg = %+v", cfg)
	}
}
