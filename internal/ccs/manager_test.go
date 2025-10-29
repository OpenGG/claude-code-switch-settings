package ccs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/claude-code-switch-settings/internal/ccs"
)

func newTestManager(t *testing.T) *ccs.Manager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCS_HOME", dir)
	mgr, err := ccs.NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}
	return mgr
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	return content
}

func TestCalculateMD5(t *testing.T) {
	mgr := newTestManager(t)
	target := filepath.Join(mgr.SettingsStoreDir(), "sample.json")
	writeFile(t, target, []byte("hello"))

	hash, err := mgr.CalculateMD5(target)
	if err != nil {
		t.Fatalf("CalculateMD5 error: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}

	emptyPath := filepath.Join(mgr.SettingsStoreDir(), "empty.json")
	writeFile(t, emptyPath, []byte{})
	hash, err = mgr.CalculateMD5(emptyPath)
	if err != nil {
		t.Fatalf("CalculateMD5 empty error: %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash for zero-byte file")
	}
}

func TestBackupFileCreatesAndUpdates(t *testing.T) {
	mgr := newTestManager(t)
	source := filepath.Join(mgr.SettingsStoreDir(), "active.json")
	writeFile(t, source, []byte("backup-data"))

	if err := mgr.BackupFile(source); err != nil {
		t.Fatalf("BackupFile error: %v", err)
	}

	hash, err := mgr.CalculateMD5(source)
	if err != nil {
		t.Fatalf("CalculateMD5 error: %v", err)
	}
	backupPath := filepath.Join(mgr.BackupDir(), hash+".json")
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	firstMod := info.ModTime()

	time.Sleep(1100 * time.Millisecond)
	if err := mgr.BackupFile(source); err != nil {
		t.Fatalf("BackupFile second run error: %v", err)
	}
	info, err = os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file missing after second run: %v", err)
	}
	if !info.ModTime().After(firstMod) {
		t.Fatalf("expected backup mtime to update")
	}
}

func TestValidateSettingsName(t *testing.T) {
	cases := []struct {
		name    string
		valid   bool
		message string
	}{
		{"my-settings_1.2", true, "valid"},
		{"my/settings", false, "invalid characters"},
		{"my设置", false, "non ascii"},
		{"CON", false, "reserved"},
		{"   ", false, "empty"},
	}

	for _, tc := range cases {
		valid, err := ccs.ValidateSettingsName(tc.name)
		if tc.valid && (!valid || err != nil) {
			t.Fatalf("expected %s to be valid, got err=%v", tc.name, err)
		}
		if !tc.valid && valid {
			t.Fatalf("expected %s to be invalid", tc.name)
		}
	}
}

func TestListSettingsActiveAndModified(t *testing.T) {
	mgr := newTestManager(t)
	activePath := mgr.ActiveSettingsPath()
	storePath := filepath.Join(mgr.SettingsStoreDir(), "work.json")
	writeFile(t, activePath, []byte("current"))
	writeFile(t, storePath, []byte("current"))
	if err := mgr.SetActiveSettings("work"); err != nil {
		t.Fatalf("SetActiveSettings error: %v", err)
	}

	entries, err := mgr.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings error: %v", err)
	}
	if len(entries) != 1 || entries[0].Display != "* [work] (active)" {
		t.Fatalf("unexpected list output: %+v", entries)
	}

	writeFile(t, activePath, []byte("changed"))
	entries, err = mgr.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings error: %v", err)
	}
	if len(entries) != 1 || entries[0].Display != "* [work] (active, modified)" {
		t.Fatalf("unexpected modified output: %+v", entries)
	}
}

func TestListSettingsHandlesMissingAndUnsaved(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.SetActiveSettings("ghost"); err != nil {
		t.Fatalf("SetActiveSettings error: %v", err)
	}

	entries, err := mgr.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings error: %v", err)
	}
	if len(entries) != 1 || entries[0].Display != "! [ghost] (active, missing!)" {
		t.Fatalf("unexpected missing output: %+v", entries)
	}

	if err := os.Remove(mgr.ActiveStatePath()); err != nil {
		t.Fatalf("remove active state error: %v", err)
	}
	writeFile(t, mgr.ActiveSettingsPath(), []byte("standalone"))

	entries, err = mgr.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings error: %v", err)
	}
	if len(entries) != 1 || entries[0].Display != "* (Current settings.json is unsaved)" {
		t.Fatalf("unexpected unsaved output: %+v", entries)
	}
}

func TestUseSettingsCopiesAndActivates(t *testing.T) {
	mgr := newTestManager(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("stored"))
	writeFile(t, mgr.ActiveSettingsPath(), []byte("old"))

	if err := mgr.UseSettings("work"); err != nil {
		t.Fatalf("UseSettings error: %v", err)
	}

	if string(readFile(t, mgr.ActiveSettingsPath())) != "stored" {
		t.Fatalf("active file not updated")
	}

	name, err := mgr.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName error: %v", err)
	}
	if name != "work" {
		t.Fatalf("expected active name 'work', got %s", name)
	}
}

func TestUseSettingsUpdatesMTime(t *testing.T) {
	mgr := newTestManager(t)
	target := filepath.Join(mgr.SettingsStoreDir(), "work.json")
	writeFile(t, target, []byte("same"))

	if err := mgr.UseSettings("work"); err != nil {
		t.Fatalf("UseSettings error: %v", err)
	}
	info, err := os.Stat(mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	first := info.ModTime()

	time.Sleep(1100 * time.Millisecond)
	if err := mgr.UseSettings("work"); err != nil {
		t.Fatalf("UseSettings second error: %v", err)
	}
	info, err = os.Stat(mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if !info.ModTime().After(first) {
		t.Fatalf("expected settings.json mtime to update")
	}
}

func TestSaveSettingsCreatesBackupAndActivates(t *testing.T) {
	mgr := newTestManager(t)
	writeFile(t, mgr.ActiveSettingsPath(), []byte("current"))

	targetPath := filepath.Join(mgr.SettingsStoreDir(), "personal.json")
	writeFile(t, targetPath, []byte("old"))

	hash, err := mgr.CalculateMD5(targetPath)
	if err != nil {
		t.Fatalf("CalculateMD5 error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := mgr.SaveSettings("personal"); err != nil {
		t.Fatalf("SaveSettings error: %v", err)
	}

	if string(readFile(t, targetPath)) != "current" {
		t.Fatalf("target file not updated")
	}

	name, err := mgr.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName error: %v", err)
	}
	if name != "personal" {
		t.Fatalf("expected active name 'personal', got %s", name)
	}

	backupPath := filepath.Join(mgr.BackupDir(), hash+".json")
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
}

func TestSaveSettingsRejectsInvalidName(t *testing.T) {
	mgr := newTestManager(t)
	writeFile(t, mgr.ActiveSettingsPath(), []byte("content"))

	if err := mgr.SaveSettings("   "); err == nil {
		t.Fatalf("expected error for empty name")
	}
}

func TestUseSettingsTrimsWhitespace(t *testing.T) {
	mgr := newTestManager(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("stored"))
	writeFile(t, mgr.ActiveSettingsPath(), []byte("old"))

	if err := mgr.UseSettings("  work  "); err != nil {
		t.Fatalf("UseSettings trim error: %v", err)
	}
	if string(readFile(t, mgr.ActiveSettingsPath())) != "stored" {
		t.Fatalf("expected trimmed name to load settings")
	}
}

func TestParseRetentionInterval(t *testing.T) {
	cases := []struct {
		input  string
		expect time.Duration
		ok     bool
	}{
		{"30d", 30 * 24 * time.Hour, true},
		{"12h", 12 * time.Hour, true},
		{"10m", 10 * time.Minute, true},
		{"5s", 5 * time.Second, true},
		{"1d6h", (24 + 6) * time.Hour, true},
		{"bad", 0, false},
	}

	for _, tc := range cases {
		result, err := ccs.ParseRetentionInterval(tc.input)
		if tc.ok && err != nil {
			t.Fatalf("expected success for %s, got %v", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("expected error for %s", tc.input)
		}
		if tc.ok && result != tc.expect {
			t.Fatalf("duration mismatch for %s: got %s want %s", tc.input, result, tc.expect)
		}
	}
}

func TestPruneBackups(t *testing.T) {
	mgr := newTestManager(t)
	oldFile := filepath.Join(mgr.BackupDir(), "old.json")
	newFile := filepath.Join(mgr.BackupDir(), "new.json")
	writeFile(t, oldFile, []byte("old"))
	writeFile(t, newFile, []byte("new"))

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	removed, err := mgr.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 file removed, got %d", removed)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("expected new file to remain: %v", err)
	}
}
