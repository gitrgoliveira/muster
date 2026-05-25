package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/config"
)

func TestResolveBeadsDir(t *testing.T) {
	t.Run("flag wins over env", func(t *testing.T) {
		dir, err := config.ResolveBeadsDir("/flag/path", "/env/path")
		if err != nil {
			t.Fatal(err)
		}
		if dir != "/flag/path" {
			t.Errorf("want /flag/path got %q", dir)
		}
	})

	t.Run("env wins over cwd", func(t *testing.T) {
		tmp := t.TempDir()
		dir, err := config.ResolveBeadsDir("", tmp)
		if err != nil {
			t.Fatal(err)
		}
		if dir != tmp {
			t.Errorf("want %q got %q", tmp, dir)
		}
	})

	t.Run("cwd fallback uses .beads subdir", func(t *testing.T) {
		tmp := t.TempDir()
		beadsDir := filepath.Join(tmp, ".beads")
		if err := os.Mkdir(beadsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Change working directory so the cwd fallback finds .beads/
		orig, _ := os.Getwd()
		if err := os.Chdir(tmp); err != nil {
			t.Skip("cannot chdir:", err)
		}
		defer os.Chdir(orig) //nolint:errcheck

		dir, err := config.ResolveBeadsDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		// Resolve symlinks on macOS (/var → /private/var).
		wantResolved, _ := filepath.EvalSymlinks(beadsDir)
		gotResolved, _ := filepath.EvalSymlinks(dir)
		if gotResolved != wantResolved {
			t.Errorf("want %q got %q", wantResolved, gotResolved)
		}
	})

	t.Run("all absent returns error", func(t *testing.T) {
		tmp := t.TempDir()
		orig, _ := os.Getwd()
		if err := os.Chdir(tmp); err != nil {
			t.Skip("cannot chdir:", err)
		}
		defer os.Chdir(orig) //nolint:errcheck

		_, err := config.ResolveBeadsDir("", "")
		if err == nil {
			t.Fatal("want error, got nil")
		}
	})
}

func TestLoadBackendConfig(t *testing.T) {
	makeDir := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("embedded mode defaults", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1}`)
		cfg, err := config.LoadBackendConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Mode != "embedded" {
			t.Errorf("mode want embedded got %q", cfg.Mode)
		}
		if cfg.ReadSource != "issues.jsonl" {
			t.Errorf("readSource want issues.jsonl got %q", cfg.ReadSource)
		}
		if cfg.BeadsDir != dir {
			t.Errorf("beadsDir want %q got %q", dir, cfg.BeadsDir)
		}
		if cfg.SchemaVersion != 1 {
			t.Errorf("schemaVersion want 1 got %d", cfg.SchemaVersion)
		}
	})

	t.Run("remote mode requires dolt_host", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1,"dolt_mode":"remote","dolt_database":"mydb"}`)
		_, err := config.LoadBackendConfig(dir)
		if err == nil {
			t.Fatal("want error for missing dolt_host")
		}
		if !strings.Contains(err.Error(), "dolt_host") {
			t.Errorf("error %q does not mention dolt_host", err.Error())
		}
	})

	t.Run("remote mode requires dolt_database", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1,"dolt_mode":"remote","dolt_host":"localhost"}`)
		_, err := config.LoadBackendConfig(dir)
		if err == nil {
			t.Fatal("want error for missing dolt_database")
		}
		if !strings.Contains(err.Error(), "dolt_database") {
			t.Errorf("error %q does not mention dolt_database", err.Error())
		}
	})

	t.Run("invalid dolt_mode", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1,"dolt_mode":"unknown"}`)
		_, err := config.LoadBackendConfig(dir)
		if err == nil {
			t.Fatal("want error for invalid dolt_mode")
		}
	})

	t.Run("BEADS_DOLT_PASSWORD env is read", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1,"dolt_mode":"remote","dolt_host":"localhost","dolt_database":"db"}`)
		t.Setenv("BEADS_DOLT_PASSWORD", "s3cr3t")

		cfg, err := config.LoadBackendConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DoltPassword != "s3cr3t" {
			t.Errorf("doltPassword want s3cr3t got %q", cfg.DoltPassword)
		}
	})

	t.Run("remote mode sets readSource dolt", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":1,"dolt_mode":"remote","dolt_host":"localhost","dolt_database":"db","dolt_user":"u"}`)
		cfg, err := config.LoadBackendConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Mode != "remote" {
			t.Errorf("mode want remote got %q", cfg.Mode)
		}
		if cfg.ReadSource != "dolt" {
			t.Errorf("readSource want dolt got %q", cfg.ReadSource)
		}
	})

	t.Run("schema_version out of range propagates error", func(t *testing.T) {
		dir := makeDir(t, `{"schema_version":99}`)
		_, err := config.LoadBackendConfig(dir)
		if err == nil {
			t.Fatal("want schema version error")
		}
		if !strings.Contains(err.Error(), "schema v99") {
			t.Errorf("error %q does not contain 'schema v99'", err.Error())
		}
	})
}
