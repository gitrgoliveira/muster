package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gitrgoliveira/muster/internal/config"
)

func writeMetadata(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMetadata(t *testing.T) {
	tests := []struct {
		name    string
		content string // "" means don't create file
		wantErr string
		check   func(t *testing.T, m *config.Metadata)
	}{
		{
			name:    "file missing",
			content: "",
			wantErr: "cannot read metadata.json",
		},
		{
			name:    "file unparseable",
			content: `{not valid json`,
			wantErr: "cannot parse metadata.json",
		},
		{
			name:    "unsupported database",
			content: `{"database":"sqlite"}`,
			wantErr: `unsupported database "sqlite"`,
		},
		{
			name:    "valid embedded no dolt_mode",
			content: `{"schema_version":1}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.SchemaVersion != 1 {
					t.Errorf("schema_version want 1 got %d", m.SchemaVersion)
				}
				if m.DoltMode != "" {
					t.Errorf("dolt_mode want empty got %q", m.DoltMode)
				}
			},
		},
		{
			name:    "valid remote with host port user",
			content: `{"schema_version":1,"dolt_mode":"remote","dolt_database":"mydb","dolt_host":"localhost","dolt_port":3306,"dolt_user":"root"}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.DoltMode != "remote" {
					t.Errorf("dolt_mode want remote got %q", m.DoltMode)
				}
				if m.DoltHost != "localhost" {
					t.Errorf("dolt_host want localhost got %q", m.DoltHost)
				}
				if m.DoltPort != 3306 {
					t.Errorf("dolt_port want 3306 got %d", m.DoltPort)
				}
				if m.DoltUser != "root" {
					t.Errorf("dolt_user want root got %q", m.DoltUser)
				}
			},
		},
		{
			name:    "schema_version absent defaults to 1",
			content: `{}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.SchemaVersion != 1 {
					t.Errorf("schema_version want 1 got %d", m.SchemaVersion)
				}
			},
		},
		{
			name:    "schema_version 2 valid",
			content: `{"schema_version":2}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.SchemaVersion != 2 {
					t.Errorf("schema_version want 2 got %d", m.SchemaVersion)
				}
			},
		},
		{
			name:    "schema_version 99 error",
			content: `{"schema_version":99}`,
			wantErr: "beads schema v99 not supported by muster (need 1..2)",
		},
		{
			name:    "database dolt explicit ok",
			content: `{"database":"dolt","schema_version":1}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.Database != "dolt" {
					t.Errorf("database want dolt got %q", m.Database)
				}
			},
		},
		{
			name:    "project_id preserved",
			content: `{"schema_version":1,"project_id":"proj-abc"}`,
			check: func(t *testing.T, m *config.Metadata) {
				if m.ProjectID != "proj-abc" {
					t.Errorf("project_id want proj-abc got %q", m.ProjectID)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.content != "" {
				writeMetadata(t, dir, tc.content)
			}

			m, err := config.LoadMetadata(dir)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if got := err.Error(); !containsStr(got, tc.wantErr) {
					t.Fatalf("error %q does not contain %q", got, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, m)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
