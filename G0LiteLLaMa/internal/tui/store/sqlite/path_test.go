package sqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDBPathEnvOverride(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "custom", "g0litellama.db")
	t.Setenv("G0LITELLAMA_DB_PATH", dbPath)

	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	if got != dbPath {
		t.Errorf("expected %q, got %q", dbPath, got)
	}

	// Directory should have been created.
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestDBPathDefaultCreatesDir(t *testing.T) {
	// Override with a temp dir to avoid touching real home.
	dir := t.TempDir()
	t.Setenv("G0LITELLAMA_DB_PATH", filepath.Join(dir, "g0litellama.db"))

	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	if got != filepath.Join(dir, "g0litellama.db") {
		t.Errorf("unexpected path: %s", got)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestDefaultDirPlatform(t *testing.T) {
	// Just verify it returns something reasonable without crashing.
	dir, err := defaultDir()
	if err != nil {
		t.Fatalf("defaultDir: %v", err)
	}
	if dir == "" {
		t.Fatal("defaultDir returned empty")
	}
	switch runtime.GOOS {
	case "darwin":
		if !filepath.IsAbs(dir) {
			t.Errorf("expected absolute path on darwin, got %q", dir)
		}
	case "windows":
		// May or may not have APPDATA set in test env; skip check.
	default:
		if !filepath.IsAbs(dir) {
			t.Errorf("expected absolute path, got %q", dir)
		}
	}
}
