package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Metadata holds the parsed content of <beads-dir>/metadata.json.
type Metadata struct {
	Database      string `json:"database"`
	DoltMode      string `json:"dolt_mode"`
	DoltDatabase  string `json:"dolt_database"`
	DoltHost      string `json:"dolt_host"`
	DoltPort      int    `json:"dolt_port"`
	DoltUser      string `json:"dolt_user"`
	SchemaVersion int    `json:"schema_version"`
	ProjectID     string `json:"project_id"`
}

// LoadMetadata reads and validates <dir>/metadata.json.
func LoadMetadata(dir string) (*Metadata, error) {
	path := filepath.Join(dir, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read metadata.json: %w", err)
	}

	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("cannot parse metadata.json: %w", err)
	}

	if m.Database != "" && m.Database != "dolt" {
		return nil, fmt.Errorf("unsupported database %q (only \"dolt\" is supported)", m.Database)
	}

	// Default schema version to 1 if absent.
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}

	return &m, nil
}
