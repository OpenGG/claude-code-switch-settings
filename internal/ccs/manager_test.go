package ccs

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	fs := afero.NewMemMapFs()
	mgr := NewManager(fs, "/home/test", nil) // nil logger = discard logger for tests
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	return mgr
}

func TestCalculateHash(t *testing.T) {
	mgr := newTestManager(t)
	path := filepath.Join(mgr.claudeDir(), "file.json")
	if err := afero.WriteFile(mgr.fs, path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	hash, err := mgr.CalculateHash(path)
	if err != nil {
		t.Fatalf("CalculateHash error: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}

	emptyPath := filepath.Join(mgr.claudeDir(), "empty.json")
	if err := afero.WriteFile(mgr.fs, emptyPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	hash, err = mgr.CalculateHash(emptyPath)
	if err != nil {
		t.Fatalf("CalculateHash empty error: %v", err)
	}
	if hash != "empty" {
		t.Fatalf("expected 'empty' hash for empty file, got %s", hash)
	}
}

func TestGetActiveSettingsName(t *testing.T) {
	mgr := newTestManager(t)
	name := mgr.GetActiveSettingsName()
	if name != "" {
		t.Fatalf("expected empty name, got %q", name)
	}

	expected := "work"
	if err := mgr.SetActiveSettings(expected); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if got := mgr.GetActiveSettingsName(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestBackupFileCreatesAndUpdates(t *testing.T) {
	mgr := newTestManager(t)
	path := mgr.ActiveSettingsPath()
	time1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time1.Add(2 * time.Second)

	mgr.SetNow(func() time.Time { return time1 })
	if err := afero.WriteFile(mgr.fs, path, []byte("content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mgr.backupFile(path); err != nil {
		t.Fatalf("backup: %v", err)
	}

	hash, err := mgr.CalculateHash(path)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	backupPath := filepath.Join(mgr.BackupDir(), hash+".json")
	info, err := mgr.fs.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if !info.ModTime().Equal(time1) {
		t.Fatalf("expected mod time %v, got %v", time1, info.ModTime())
	}

	mgr.SetNow(func() time.Time { return time2 })
	if err := afero.WriteFile(mgr.fs, path, []byte("content"), 0o644); err != nil {
		t.Fatalf("write again: %v", err)
	}
	if err := mgr.backupFile(path); err != nil {
		t.Fatalf("backup update: %v", err)
	}
	info, err = mgr.fs.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup second: %v", err)
	}
	if !info.ModTime().Equal(time2) {
		t.Fatalf("expected mod time %v, got %v", time2, info.ModTime())
	}
}

func TestValidateSettingsName(t *testing.T) {
	mgr := newTestManager(t)
	ok, err := mgr.ValidateSettingsName("my-settings_1.2")
	if !ok || err != nil {
		t.Fatalf("expected valid name, got err %v", err)
	}

	invalids := []string{"my/settings", "my设置", "CON", " ", ".."}
	for _, val := range invalids {
		ok, err := mgr.ValidateSettingsName(val)
		if ok || err == nil {
			t.Fatalf("expected invalid name for %q", val)
		}
	}
}

func TestListSettingsStates(t *testing.T) {
	mgr := newTestManager(t)
	store := mgr.SettingsStoreDir()
	if err := afero.WriteFile(mgr.fs, filepath.Join(store, "work.json"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write work: %v", err)
	}
	if err := mgr.SetActiveSettings("work"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("A"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	entries, err := mgr.ListSettings()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Prefix != "*" || !contains(entries[0].Qualifiers, "active") {
		t.Fatalf("expected active entry: %+v", entries[0])
	}

	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("B"), 0o644); err != nil {
		t.Fatalf("write modified: %v", err)
	}
	entries, err = mgr.ListSettings()
	if err != nil {
		t.Fatalf("list modified: %v", err)
	}
	if !contains(entries[0].Qualifiers, "modified") {
		t.Fatalf("expected modified qualifier: %+v", entries[0])
	}

	if err := mgr.SetActiveSettings("ghost"); err != nil {
		t.Fatalf("set ghost: %v", err)
	}
	entries, err = mgr.ListSettings()
	if err != nil {
		t.Fatalf("list ghost: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "ghost" && contains(e.Qualifiers, "missing!") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing entry, got %+v", entries)
	}

	if err := mgr.SetActiveSettings(""); err != nil {
		t.Fatalf("clear active: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("C"), 0o644); err != nil {
		t.Fatalf("write unsaved: %v", err)
	}
	entries, err = mgr.ListSettings()
	if err != nil {
		t.Fatalf("list unsaved: %v", err)
	}
	found = false
	for _, e := range entries {
		if e.Plain && e.Name == "(Current settings.json is unsaved)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsaved entry, got %+v", entries)
	}
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func TestUseSwitchesSettingsAndUpdatesTimestamp(t *testing.T) {
	mgr := newTestManager(t)
	store := mgr.SettingsStoreDir()
	if err := afero.WriteFile(mgr.fs, filepath.Join(store, "work.json"), []byte("stored"), 0o644); err != nil {
		t.Fatalf("write work: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("current"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}
	time1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	time2 := time1.Add(1 * time.Second)

	mgr.SetNow(func() time.Time { return time1 })
	if err := mgr.Use("work"); err != nil {
		t.Fatalf("use work: %v", err)
	}
	mgr.SetNow(func() time.Time { return time2 })
	if err := mgr.Use("work"); err != nil {
		t.Fatalf("use work second: %v", err)
	}
	info, err := mgr.fs.Stat(mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("stat active: %v", err)
	}
	if info.ModTime().Before(time2) {
		t.Fatalf("expected mod time update, got %v", info.ModTime())
	}
	content, err := afero.ReadFile(mgr.fs, mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(content) != "stored" {
		t.Fatalf("expected stored content, got %s", content)
	}
	if mgr.GetActiveSettingsName() != "work" {
		t.Fatalf("expected active name 'work'")
	}
}

func TestSaveOverwritesStoredSettings(t *testing.T) {
	mgr := newTestManager(t)
	store := mgr.SettingsStoreDir()
	if err := afero.WriteFile(mgr.fs, filepath.Join(store, "personal.json"), []byte("initial"), 0o644); err != nil {
		t.Fatalf("write personal: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("Mod"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.Save("personal"); err != nil {
		t.Fatalf("save: %v", err)
	}
	content, err := afero.ReadFile(mgr.fs, filepath.Join(store, "personal.json"))
	if err != nil {
		t.Fatalf("read personal: %v", err)
	}
	if string(content) != "Mod" {
		t.Fatalf("expected updated content, got %s", content)
	}
	if mgr.GetActiveSettingsName() != "personal" {
		t.Fatalf("expected active name 'personal'")
	}
}

func TestSaveCreatesNewSettings(t *testing.T) {
	mgr := newTestManager(t)
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("data"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.Save("dev"); err != nil {
		t.Fatalf("save new: %v", err)
	}
	path, err := mgr.StoredSettingsPath("dev")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	content, err := afero.ReadFile(mgr.fs, path)
	if err != nil {
		t.Fatalf("read dev: %v", err)
	}
	if string(content) != "data" {
		t.Fatalf("expected stored data, got %s", content)
	}
}

func TestUseRejectsInvalidName(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Use("../bad"); !errors.Is(err, ErrSettingsNameInvalidChars) {
		t.Fatalf("expected invalid character error, got %v", err)
	}
}

func TestSaveRejectsInvalidName(t *testing.T) {
	mgr := newTestManager(t)
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("data"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.Save("../bad"); !errors.Is(err, ErrSettingsNameInvalidChars) {
		t.Fatalf("expected invalid character error, got %v", err)
	}
	escapePath := filepath.Join(mgr.SettingsStoreDir(), "..", "bad.json")
	exists, err := afero.Exists(mgr.fs, escapePath)
	if err != nil {
		t.Fatalf("exists escape path: %v", err)
	}
	if exists {
		t.Fatalf("unexpected file created outside store")
	}
}

func TestStoredSettingsPathInvalidName(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.StoredSettingsPath("../bad"); !errors.Is(err, ErrSettingsNameInvalidChars) {
		t.Fatalf("expected invalid character error, got %v", err)
	}
}

func TestSaveTrimsSettingsName(t *testing.T) {
	mgr := newTestManager(t)
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("data"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.Save(" dev "); err != nil {
		t.Fatalf("save with spaces: %v", err)
	}
	if mgr.GetActiveSettingsName() != "dev" {
		t.Fatalf("expected trimmed active name, got %q", mgr.GetActiveSettingsName())
	}
	path, err := mgr.StoredSettingsPath("dev")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	exists, err := afero.Exists(mgr.fs, path)
	if err != nil {
		t.Fatalf("exists dev: %v", err)
	}
	if !exists {
		t.Fatalf("expected stored settings file for dev")
	}
}

func TestPruneBackups(t *testing.T) {
	mgr := newTestManager(t)
	backup := mgr.BackupDir()
	time1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time1.Add(30 * time.Hour)

	files := []struct {
		name string
		mod  time.Time
	}{
		{"old.json", time1},
		{"recent.json", time2},
	}

	for _, f := range files {
		path := filepath.Join(backup, f.name)
		if err := afero.WriteFile(mgr.fs, path, []byte("backup"), 0o644); err != nil {
			t.Fatalf("write backup: %v", err)
		}
		if err := mgr.fs.Chtimes(path, f.mod, f.mod); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	mgr.SetNow(func() time.Time { return time1.Add(48 * time.Hour) })
	deleted, err := mgr.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	exists, err := afero.Exists(mgr.fs, filepath.Join(backup, "old.json"))
	if err != nil {
		t.Fatalf("exists old: %v", err)
	}
	if exists {
		t.Fatalf("old backup should be removed")
	}

	exists, err = afero.Exists(mgr.fs, filepath.Join(backup, "recent.json"))
	if err != nil {
		t.Fatalf("exists recent: %v", err)
	}
	if !exists {
		t.Fatalf("recent backup should stay")
	}
}

func TestCalculateHashMissingFile(t *testing.T) {
	mgr := newTestManager(t)
	hash, err := mgr.CalculateHash(filepath.Join(mgr.claudeDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected no error for missing file: %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash for missing file")
	}
}

func TestCopyFileOverwritesDestination(t *testing.T) {
	mgr := newTestManager(t)
	src := filepath.Join(mgr.claudeDir(), "src.json")
	dst := filepath.Join(mgr.claudeDir(), "dst.json")
	if err := afero.WriteFile(mgr.fs, src, []byte("first"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	if err := mgr.copyFile(src, dst); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	content, err := afero.ReadFile(mgr.fs, dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(content) != "first" {
		t.Fatalf("expected copied content, got %s", content)
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.copyFile(filepath.Join(mgr.claudeDir(), "missing"), filepath.Join(mgr.claudeDir(), "dst")); err == nil {
		t.Fatalf("expected error for missing source")
	}
}

func TestUseMissingSettings(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Use("ghost"); err == nil {
		t.Fatalf("expected error for missing settings")
	}
}

func TestSaveWithoutActiveFile(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Save("test"); err == nil {
		t.Fatalf("expected error when settings.json missing")
	}
}

func TestSetNowNilReset(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetNow(nil)
	if mgr.now == nil {
		t.Fatalf("expected now function to be set")
	}
}

func TestAccessors(t *testing.T) {
	mgr := newTestManager(t)
	if mgr.ActiveStatePath() == "" {
		t.Fatalf("active state path should not be empty")
	}
	if mgr.FileSystem() == nil {
		t.Fatalf("filesystem accessor should not return nil")
	}
}

func TestValidateSettingsNameBoundaries(t *testing.T) {
	mgr := newTestManager(t)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid cases
		{"simple name", "work", false},
		{"with hyphen", "my-settings", false},
		{"with underscore", "my_settings", false},
		{"with numbers", "settings123", false},
		{"with dots", "v1.2.3", false},
		{"max ASCII", "test-~", false},

		// Invalid cases - empty and whitespace
		{"empty string", "", true},
		{"only spaces", "   ", true},
		{"only tab", "\t", true},

		// Invalid cases - dot navigation
		{"single dot", ".", true},
		{"double dot", "..", true},

		// Invalid cases - null bytes
		{"null byte at start", "\x00test", true},
		{"null byte in middle", "test\x00file", true},
		{"null byte at end", "test\x00", true},

		// Invalid cases - control characters
		{"control char NUL", "test\x00", true},
		{"control char SOH", "test\x01", true},
		{"control char STX", "test\x02", true},
		{"control char DEL", "test\x7f", true},
		{"newline", "test\nfile", true},
		{"carriage return", "test\rfile", true},
		{"tab character", "test\tfile", true},

		// Invalid cases - non-printable ASCII
		{"below space", "test\x1f", true},
		{"above tilde", "test\x80", true},

		// Invalid cases - invalid filesystem characters
		{"forward slash", "my/settings", true},
		{"backslash", "my\\settings", true},
		{"colon", "my:settings", true},
		{"asterisk", "my*settings", true},
		{"question mark", "my?settings", true},
		{"double quote", "my\"settings", true},
		{"less than", "my<settings", true},
		{"greater than", "my>settings", true},
		{"pipe", "my|settings", true},

		// Invalid cases - reserved Windows names
		{"CON uppercase", "CON", true},
		{"CON lowercase", "con", true},
		{"CON mixed case", "Con", true},
		{"PRN", "PRN", true},
		{"AUX", "AUX", true},
		{"NUL", "NUL", true},
		{"COM1", "COM1", true},
		{"COM9", "COM9", true},
		{"LPT1", "LPT1", true},
		{"LPT9", "LPT9", true},

		// Invalid cases - Unicode (non-ASCII)
		{"unicode emoji", "settings😀", true},
		{"unicode Chinese", "设置", true},
		{"unicode accented", "café", true},
		{"unicode Cyrillic", "настройки", true},

		// Edge cases - length (filesystem limits vary, but these should work)
		{"very long name", string(make([]byte, 255)), false}, // All spaces become single space when trimmed
		{"255 a's", string(make([]rune, 255)), false},

		// Whitespace handling
		{"leading spaces", "  work", false},  // Trimmed to "work"
		{"trailing spaces", "work  ", false}, // Trimmed to "work"
		{"both spaces", "  work  ", false},   // Trimmed to "work"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Special handling for the very long name test
			input := tt.input
			if tt.name == "very long name" {
				// Create a valid 255-character name
				for i := range input {
					input = input[:i] + "a"
				}
			}
			if tt.name == "255 a's" {
				input = ""
				for i := 0; i < 255; i++ {
					input += "a"
				}
			}

			valid, err := mgr.ValidateSettingsName(input)
			if tt.wantErr {
				if valid || err == nil {
					t.Errorf("expected error for %q, got valid=%v err=%v", input, valid, err)
				}
			} else {
				if !valid || err != nil {
					t.Errorf("unexpected error for %q: valid=%v err=%v", input, valid, err)
				}
			}
		})
	}
}

func TestValidateSettingsNameNullByte(t *testing.T) {
	mgr := newTestManager(t)

	// Explicit null byte test
	valid, err := mgr.ValidateSettingsName("test\x00file")
	if valid {
		t.Error("expected invalid for null byte")
	}
	if !errors.Is(err, ErrSettingsNameNullByte) {
		t.Errorf("expected ErrSettingsNameNullByte, got %v", err)
	}
}

func TestPruneBackupsNoDeletion(t *testing.T) {
	mgr := newTestManager(t)
	backup := mgr.BackupDir()
	path := filepath.Join(backup, "recent.json")
	if err := afero.WriteFile(mgr.fs, path, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write recent: %v", err)
	}
	mgr.SetNow(func() time.Time { return time.Now() })
	deleted, err := mgr.PruneBackups(72 * time.Hour)
	if err != nil {
		t.Fatalf("prune error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected no deletions, got %d", deleted)
	}
}

func TestBackupFileCreatesBackupForEmptyFile(t *testing.T) {
	mgr := newTestManager(t)
	path := mgr.ActiveSettingsPath()
	if err := afero.WriteFile(mgr.fs, path, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if err := mgr.backupFile(path); err != nil {
		t.Fatalf("backup empty: %v", err)
	}
	entries, err := afero.ReadDir(mgr.fs, mgr.BackupDir())
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	// Empty files now create a backup with hash "empty"
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup for empty file, got %d", len(entries))
	}
	if entries[0].Name() != "empty.json" {
		t.Fatalf("expected backup named 'empty.json', got %s", entries[0].Name())
	}
}

func TestBackupFileMissingSource(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.backupFile(filepath.Join(mgr.claudeDir(), "missing.json")); err != nil {
		t.Fatalf("expected no error for missing file: %v", err)
	}
}

func TestUseCreatesBackup(t *testing.T) {
	mgr := newTestManager(t)
	store := mgr.SettingsStoreDir()
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("current"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, filepath.Join(store, "work.json"), []byte("stored"), 0o644); err != nil {
		t.Fatalf("write work: %v", err)
	}
	if err := mgr.Use("work"); err != nil {
		t.Fatalf("use work: %v", err)
	}
	files, err := afero.ReadDir(mgr.fs, mgr.BackupDir())
	if err != nil {
		t.Fatalf("read backups: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected backup file")
	}
}

func TestSaveCreatesBackupOfTarget(t *testing.T) {
	mgr := newTestManager(t)
	store := mgr.SettingsStoreDir()
	if err := afero.WriteFile(mgr.fs, filepath.Join(store, "personal.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write personal: %v", err)
	}
	if err := afero.WriteFile(mgr.fs, mgr.ActiveSettingsPath(), []byte("new"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.Save("personal"); err != nil {
		t.Fatalf("save: %v", err)
	}
	files, err := afero.ReadDir(mgr.fs, mgr.BackupDir())
	if err != nil {
		t.Fatalf("read backups: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected backup file for personal")
	}
}

func TestPruneBackupsIgnoresDirectories(t *testing.T) {
	mgr := newTestManager(t)
	backup := mgr.BackupDir()
	if err := mgr.fs.MkdirAll(filepath.Join(backup, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	mgr.SetNow(func() time.Time { return time.Now() })
	deleted, err := mgr.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected no deletions when only directories present")
	}
}

func TestInitInfraError(t *testing.T) {
	roFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	mgr := NewManager(roFs, "/home/ro", nil)
	if err := mgr.InitInfra(); err == nil {
		t.Fatalf("expected error initializing read-only fs")
	}
}
