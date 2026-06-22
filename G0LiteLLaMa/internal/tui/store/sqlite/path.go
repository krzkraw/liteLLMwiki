package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// envOverride is checked first. If set, its value is used as the DB path
// directly. Tests and portable/debug runs set G0LITELLAMA_DB_PATH.
const envOverride = "G0LITELLAMA_DB_PATH"

// defaultDir returns the platform-specific application data directory for
// G0LiteLLaMa.
func defaultDir() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home dir: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "G0LiteLLaMa"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA not set")
		}
		return filepath.Join(appData, "G0LiteLLaMa"), nil
	default: // linux and others
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home dir: %w", err)
		}
		return filepath.Join(home, ".config", "g0litellama"), nil
	}
}

// DBPath returns the database file path. It checks G0LITELLAMA_DB_PATH first,
// then falls back to the platform default. The parent directory is created if
// it does not exist.
func DBPath() (string, error) {
	if p := os.Getenv(envOverride); p != "" {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create db dir %s: %w", dir, err)
		}
		return p, nil
	}

	dir, err := defaultDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create db dir %s: %w", dir, err)
	}
	return filepath.Join(dir, "g0litellama.db"), nil
}
