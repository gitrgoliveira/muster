// Package config loads beads directory configuration and backend selection.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-sql-driver/mysql"
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
	mc := mysql.Config{
		User:                 user,
		Passwd:               cfg.DoltPassword,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", host, port),
		DBName:               cfg.DoltDatabase,
		ParseTime:            true,
		Collation:            "utf8mb4_0900_ai_ci",
		AllowNativePasswords: true,
	}
	return mc.FormatDSN()
}

// BackendConfig carries resolved backend settings.
type BackendConfig struct {
	Mode          string // "embedded" or "remote"
	BeadsDir      string
	ProjectID     string
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
// Priority: flag > env (BEADS_DIR) > "./.beads" in cwd (only if it exists).
// Returns an error if none of these locate a beads directory.
func ResolveBeadsDir(flagVal, envVal string) (string, error) {
	var raw string
	switch {
	case flagVal != "":
		raw = flagVal
	case envVal != "":
		raw = envVal
	default:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot determine cwd: %w", err)
		}
		candidate := filepath.Join(cwd, ".beads")
		if _, err := os.Stat(candidate); err == nil {
			raw = candidate
		} else {
			return "", fmt.Errorf("no beads directory found; set --beads-dir or BEADS_DIR")
		}
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path for %q: %w", raw, err)
	}
	return abs, nil
}

// LoadBackendConfig loads and validates configuration from the beads directory.
func LoadBackendConfig(dir string) (BackendConfig, error) {
	m, err := LoadMetadata(dir)
	if err != nil {
		return BackendConfig{}, err
	}

	cfg := BackendConfig{
		BeadsDir:      dir,
		ProjectID:     m.ProjectID,
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
		cfg.ReadSource = "dolt-sql"
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
