// Package config loads beads directory configuration and backend selection.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// BuildDoltDSN constructs a MySQL DSN for Dolt from BackendConfig.
func BuildDoltDSN(cfg BackendConfig) string {
	host := cfg.DoltHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.DoltPort
	if port == 0 {
		port = 3306
	}
	user := cfg.DoltUser
	if user == "" {
		user = "root"
	}
	if cfg.DoltPassword != "" {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&collation=utf8mb4_0900_ai_ci",
			user, cfg.DoltPassword, host, port, cfg.DoltDatabase)
	}
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?parseTime=true&collation=utf8mb4_0900_ai_ci",
		user, host, port, cfg.DoltDatabase)
}

// BackendConfig carries resolved backend settings.
type BackendConfig struct {
	Mode          string // "embedded" or "remote"
	BeadsDir      string
	DoltDatabase  string
	DoltHost      string
	DoltPort      int
	DoltUser      string
	DoltPassword  string
	BdBin         string
	ReadSource    string
	SchemaVersion int
}

// ResolveBeadsDir resolves the beads directory from flag, env, or cwd fallback.
// Priority: flag > env (BEADS_DIR) > "./.beads/" in cwd.
func ResolveBeadsDir(flagVal, envVal string) (string, error) {
	if flagVal != "" {
		return filepath.Clean(flagVal), nil
	}
	if envVal != "" {
		return filepath.Clean(envVal), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine cwd: %w", err)
	}
	candidate := filepath.Join(cwd, ".beads")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("no beads directory found; set --beads-dir or BEADS_DIR")
}

// LoadBackendConfig loads and validates configuration from the beads directory.
func LoadBackendConfig(dir string) (BackendConfig, error) {
	m, err := LoadMetadata(dir)
	if err != nil {
		return BackendConfig{}, err
	}

	cfg := BackendConfig{
		BeadsDir:      dir,
		DoltDatabase:  m.DoltDatabase,
		DoltHost:      m.DoltHost,
		DoltPort:      m.DoltPort,
		DoltUser:      m.DoltUser,
		DoltPassword:  os.Getenv("BEADS_DOLT_PASSWORD"),
		SchemaVersion: m.SchemaVersion,
	}

	switch m.DoltMode {
	case "embedded", "":
		cfg.Mode = "embedded"
		cfg.ReadSource = "issues.jsonl"
	case "remote":
		cfg.Mode = "remote"
		cfg.ReadSource = "dolt"
		if cfg.DoltHost == "" {
			return BackendConfig{}, fmt.Errorf("dolt_host is required in remote mode")
		}
		if cfg.DoltDatabase == "" {
			return BackendConfig{}, fmt.Errorf("dolt_database is required in remote mode")
		}
	default:
		return BackendConfig{}, fmt.Errorf("invalid dolt_mode %q (want embedded or remote)", m.DoltMode)
	}

	return cfg, nil
}
