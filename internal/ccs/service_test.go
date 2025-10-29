package ccs

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	base := filepath.Join(t.TempDir(), ".claude-test")
	fs := afero.NewOsFs()
	service, err := NewService(fs, base)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	return service
}

type failingFs struct {
	afero.Fs
	failRename bool
}

func (f *failingFs) Rename(oldname, newname string) error {
	if f.failRename {
		return errors.New("rename failure")
	}
	return f.Fs.Rename(oldname, newname)
}

func TestValidateSettingsName(t *testing.T) {
	tests := []struct {
		name    string
		valid   bool
		message string
	}{
		{"my-settings_1.2", true, ""},
		{"my/settings", false, "invalid characters"},
		{"my设置", false, "non-printable"},
		{"CON", false, "reserved"},
		{"   ", false, "empty"},
	}

	for _, tt := range tests {
		valid, err := ValidateSettingsName(tt.name)
		if tt.valid && !valid {
			t.Fatalf("expected %s to be valid: %v", tt.name, err)
		}
		if !tt.valid {
			if valid {
				t.Fatalf("expected %s to be invalid", tt.name)
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.message) {
				t.Fatalf("expected error for %s to contain %s, got %v", tt.name, tt.message, err)
			}
		}
	}
}

func TestCalculateMD5AndBackup(t *testing.T) {
	service := newTestService(t)
	paths := service.Paths()

	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	content := []byte("first version")
	if err := afero.WriteFile(service.fs, filepath.Join(paths.BaseDir, "file.json"), content, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := service.BackupFile(filepath.Join(paths.BaseDir, "file.json")); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	files, err := afero.ReadDir(service.fs, paths.BackupDir)
	if err != nil {
		t.Fatalf("reading backup dir failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(files))
	}

	firstInfo, err := service.fs.Stat(filepath.Join(paths.BackupDir, files[0].Name()))
	if err != nil {
		t.Fatalf("stat backup failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	newTimestamp := firstInfo.ModTime().Add(2 * time.Second)
	service.nowFunc = func() time.Time { return newTimestamp }

	if err := service.BackupFile(filepath.Join(paths.BaseDir, "file.json")); err != nil {
		t.Fatalf("backup update failed: %v", err)
	}

	secondInfo, err := service.fs.Stat(filepath.Join(paths.BackupDir, files[0].Name()))
	if err != nil {
		t.Fatalf("stat backup failed: %v", err)
	}

	if !secondInfo.ModTime().After(firstInfo.ModTime()) {
		t.Fatalf("expected mod time to increase")
	}
}

func TestNewServiceValidation(t *testing.T) {
	if _, err := NewService(nil, "/tmp"); err == nil {
		t.Fatalf("expected error when fs is nil")
	}
	if _, err := NewService(afero.NewOsFs(), ""); err == nil {
		t.Fatalf("expected error when base dir empty")
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	service := newTestService(t)
	if err := service.copyFile("/nonexistent", filepath.Join(service.Paths().BaseDir, "out.json")); err != nil {
		t.Fatalf("copyFile should ignore missing source: %v", err)
	}
}

func TestGetAndSetActiveSettingsName(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	name, err := service.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName failed: %v", err)
	}
	if name != "" {
		t.Fatalf("expected empty active name")
	}

	if err := service.SetActiveSettings("work"); err != nil {
		t.Fatalf("SetActiveSettings failed: %v", err)
	}

	name, err = service.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName failed: %v", err)
	}
	if name != "work" {
		t.Fatalf("expected active name 'work', got %s", name)
	}
}

func TestListSettingsStates(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	paths := service.Paths()

	if err := afero.WriteFile(service.fs, filepath.Join(paths.SettingsStoreDir, "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("write work.json failed: %v", err)
	}
	if err := afero.WriteFile(service.fs, filepath.Join(paths.SettingsStoreDir, "personal.json"), []byte("personal"), 0o644); err != nil {
		t.Fatalf("write personal.json failed: %v", err)
	}

	if err := afero.WriteFile(service.fs, paths.ActiveSettingsPath, []byte("work"), 0o644); err != nil {
		t.Fatalf("write active settings failed: %v", err)
	}
	if err := service.SetActiveSettings("work"); err != nil {
		t.Fatalf("SetActiveSettings failed: %v", err)
	}

	entries, err := service.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if !contains(entries, "* [work] (active)") {
		t.Fatalf("expected work to be marked active: %v", entries)
	}
	if !contains(entries, "[personal]") {
		t.Fatalf("expected personal entry: %v", entries)
	}

	if err := service.fs.Remove(filepath.Join(paths.SettingsStoreDir, "work.json")); err != nil {
		t.Fatalf("failed to remove work.json: %v", err)
	}

	entries, err = service.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings failed: %v", err)
	}

	if !contains(entries, "missing!") {
		t.Fatalf("expected missing entry in list: %v", entries)
	}

	if err := service.SetActiveSettings(""); err != nil {
		t.Fatalf("failed to clear active state: %v", err)
	}
	if err := afero.WriteFile(service.fs, paths.ActiveSettingsPath, []byte("modified"), 0o644); err != nil {
		t.Fatalf("failed to modify active settings: %v", err)
	}

	entries, err = service.ListSettings()
	if err != nil {
		t.Fatalf("ListSettings failed: %v", err)
	}

	if !contains(entries, "unsaved") {
		t.Fatalf("expected unsaved entry: %v", entries)
	}
}

func TestUseSettings(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	paths := service.Paths()

	if err := afero.WriteFile(service.fs, filepath.Join(paths.SettingsStoreDir, "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to write work.json: %v", err)
	}

	if err := afero.WriteFile(service.fs, paths.ActiveSettingsPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	if err := service.UseSettings("work"); err != nil {
		t.Fatalf("UseSettings failed: %v", err)
	}

	content, err := afero.ReadFile(service.fs, paths.ActiveSettingsPath)
	if err != nil {
		t.Fatalf("failed to read active settings: %v", err)
	}
	if string(content) != "work" {
		t.Fatalf("expected 'work', got %s", content)
	}

	name, err := service.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName failed: %v", err)
	}
	if name != "work" {
		t.Fatalf("expected active name 'work', got %s", name)
	}
}

func TestUseSettingsMissing(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := service.UseSettings("missing"); err == nil {
		t.Fatalf("expected error when settings missing")
	}
}

func TestSaveSettings(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	paths := service.Paths()

	if err := afero.WriteFile(service.fs, paths.ActiveSettingsPath, []byte("current"), 0o644); err != nil {
		t.Fatalf("failed to write active settings: %v", err)
	}

	if err := service.SaveSettings("dev"); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	content, err := afero.ReadFile(service.fs, filepath.Join(paths.SettingsStoreDir, "dev.json"))
	if err != nil {
		t.Fatalf("failed to read saved settings: %v", err)
	}
	if string(content) != "current" {
		t.Fatalf("expected saved content 'current', got %s", content)
	}

	name, err := service.GetActiveSettingsName()
	if err != nil {
		t.Fatalf("GetActiveSettingsName failed: %v", err)
	}
	if name != "dev" {
		t.Fatalf("expected active name 'dev', got %s", name)
	}
}

func TestSaveSettingsMissingActive(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := service.SaveSettings("dev"); err == nil {
		t.Fatalf("expected error when settings.json missing")
	}
}

func TestValidateSettingsNameReservedDevices(t *testing.T) {
	reserved := []string{"COM1", "LPT2"}
	for _, name := range reserved {
		valid, err := ValidateSettingsName(name)
		if valid || err == nil {
			t.Fatalf("expected reserved name %s to be invalid", name)
		}
	}
}

func TestListStoredNamesIgnoresNonJSON(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	paths := service.Paths()
	if err := afero.WriteFile(service.fs, filepath.Join(paths.SettingsStoreDir, "notes.txt"), []byte("text"), 0o644); err != nil {
		t.Fatalf("failed to write notes.txt: %v", err)
	}
	if err := service.fs.Mkdir(filepath.Join(paths.SettingsStoreDir, "nested"), 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := afero.WriteFile(service.fs, filepath.Join(paths.SettingsStoreDir, "valid.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write valid.json: %v", err)
	}

	names, err := service.ListStoredNames()
	if err != nil {
		t.Fatalf("ListStoredNames failed: %v", err)
	}
	if len(names) != 1 || names[0] != "valid" {
		t.Fatalf("expected only valid.json to be returned: %v", names)
	}
}

func TestInitInfraReadOnly(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".claude-ro")
	mem := afero.NewMemMapFs()
	ro := afero.NewReadOnlyFs(mem)
	service, err := NewService(ro, base)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if err := service.InitInfra(); err == nil {
		t.Fatalf("expected error when infra cannot be created")
	}
}

func TestBackupFileMissingSource(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := service.BackupFile(filepath.Join(service.Paths().BaseDir, "does-not-exist.json")); err != nil {
		t.Fatalf("expected missing source to be ignored: %v", err)
	}
}

func TestCopyFileRenameFailure(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".claude")
	fs := &failingFs{Fs: afero.NewMemMapFs(), failRename: true}
	service, err := NewService(fs, base)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if err := afero.WriteFile(fs, filepath.Join(base, "source.json"), []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	if err := service.copyFile(filepath.Join(base, "source.json"), filepath.Join(base, "dest.json")); err == nil {
		t.Fatalf("expected rename failure to propagate")
	}
}

func TestUseSettingsCopyFailure(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".claude")
	fs := &failingFs{Fs: afero.NewMemMapFs(), failRename: true}
	service, err := NewService(fs, base)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	fs.failRename = false
	if err := afero.WriteFile(fs, filepath.Join(base, "switch-settings", "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}
	if err := afero.WriteFile(fs, filepath.Join(base, "settings.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write active settings: %v", err)
	}
	fs.failRename = true
	if err := service.UseSettings("work"); err == nil {
		t.Fatalf("expected error when copy fails")
	}
}

func TestSaveSettingsCopyFailure(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".claude")
	fs := &failingFs{Fs: afero.NewMemMapFs(), failRename: true}
	service, err := NewService(fs, base)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fs, filepath.Join(base, "settings.json"), []byte("current"), 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}
	if err := service.SaveSettings("dev"); err == nil {
		t.Fatalf("expected error when copy fails")
	}
}

func TestPruneBackups(t *testing.T) {
	service := newTestService(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	paths := service.Paths()

	now := time.Now()
	service.nowFunc = func() time.Time { return now }

	createBackup := func(name string, modTime time.Time) {
		path := filepath.Join(paths.BackupDir, name)
		if err := afero.WriteFile(service.fs, path, []byte("data"), 0o644); err != nil {
			t.Fatalf("failed to write backup: %v", err)
		}
		if err := service.fs.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("failed to set modtime: %v", err)
		}
	}

	createBackup("old.json", now.Add(-90*24*time.Hour))
	createBackup("recent.json", now.Add(-10*24*time.Hour))

	removed, err := service.PruneBackups(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed backup, got %d", removed)
	}

	files, err := afero.ReadDir(service.fs, paths.BackupDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(files) != 1 || files[0].Name() != "recent.json" {
		t.Fatalf("unexpected files after prune: %v", files)
	}
}

func contains(list []string, needle string) bool {
	for _, item := range list {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
