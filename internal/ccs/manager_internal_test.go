package ccs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveClaudeDirPrefersEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom")
	t.Setenv("CCS_HOME", dir)
	resolved, err := resolveClaudeDir()
	if err != nil {
		t.Fatalf("resolveClaudeDir error: %v", err)
	}
	if resolved != dir {
		t.Fatalf("expected %s, got %s", dir, resolved)
	}
}

func TestResolveClaudeDirDefaultsToHome(t *testing.T) {
	t.Setenv("CCS_HOME", "")
	resolved, err := resolveClaudeDir()
	if err != nil {
		t.Fatalf("resolveClaudeDir error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error: %v", err)
	}
	expected := filepath.Join(home, ".claude")
	if resolved != expected {
		t.Fatalf("expected %s, got %s", expected, resolved)
	}
}

func TestUseSettingsErrorPaths(t *testing.T) {
	t.Setenv("CCS_HOME", t.TempDir())
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}

	if err := mgr.UseSettings(""); err == nil {
		t.Fatalf("expected error for empty name")
	}
	if err := mgr.UseSettings("missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestSaveSettingsRequiresActiveFile(t *testing.T) {
	t.Setenv("CCS_HOME", t.TempDir())
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}

	if err := mgr.SaveSettings("new"); err == nil || !strings.Contains(err.Error(), "settings.json not found") {
		t.Fatalf("expected missing settings error, got %v", err)
	}
}

func TestBackupFileSkipsMissingSource(t *testing.T) {
	t.Setenv("CCS_HOME", t.TempDir())
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}

	if err := mgr.BackupFile(filepath.Join(mgr.SettingsStoreDir(), "absent.json")); err != nil {
		t.Fatalf("expected no error when backing up missing file, got %v", err)
	}
}

func TestPruneBackupsSkipsDirectories(t *testing.T) {
	t.Setenv("CCS_HOME", t.TempDir())
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}
	dir := filepath.Join(mgr.BackupDir(), "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	removed, err := mgr.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected 0 removals, got %d", removed)
	}
}

func TestCopyFileErrorPaths(t *testing.T) {
	missingSource := filepath.Join(t.TempDir(), "missing.json")
	if err := copyFile(missingSource, filepath.Join(t.TempDir(), "dest.json")); err == nil {
		t.Fatalf("expected error for missing source")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	dest := filepath.Join(dir, "sub", "dest.json")
	if err := copyFile(src, dest); err == nil {
		t.Fatalf("expected error for missing destination directory")
	}
}
