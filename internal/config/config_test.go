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
