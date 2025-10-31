package settings

// Tests for settings state management and display logic.
//
// Focus: ListStored (filtering, sorting), ListEntries (5-state machine for active/
// modified/missing/unsaved/inactive profiles).
//
// Note: Simple getters/setters tested via integration tests.

import (
	"path/filepath"
	"testing"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/storage"
	"github.com/spf13/afero"
)

func newTestService(t *testing.T) (*Service, afero.Fs) {
	t.Helper()
	fs := afero.NewMemMapFs()
	stor := storage.New(fs)
	settingsStore := "/store"
	activeState := "/state/active.txt"

	if err := fs.MkdirAll(settingsStore, 0o700); err != nil {
		t.Fatalf("setup store: %v", err)
	}
	if err := fs.MkdirAll("/state", 0o700); err != nil {
		t.Fatalf("setup state dir: %v", err)
	}

	return New(stor, settingsStore, activeState), fs
}

func TestListStored_Empty(t *testing.T) {
	svc, _ := newTestService(t)

	names, err := svc.ListStored()
	if err != nil {
		t.Fatalf("ListStored failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestListStored_MultipleFiles(t *testing.T) {
	svc, fs := newTestService(t)

	files := []string{"work.json", "personal.json", "dev.json"}
	for _, file := range files {
		path := filepath.Join("/store", file)
		if err := afero.WriteFile(fs, path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("create %s: %v", file, err)
		}
	}

	names, err := svc.ListStored()
	if err != nil {
		t.Fatalf("ListStored failed: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}

	// Should be sorted
	expected := []string{"dev", "personal", "work"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestListStored_IgnoresNonJsonFiles(t *testing.T) {
	svc, fs := newTestService(t)

	files := []string{"work.json", "readme.txt", "config.yaml"}
	for _, file := range files {
		path := filepath.Join("/store", file)
		if err := afero.WriteFile(fs, path, []byte("data"), 0o644); err != nil {
			t.Fatalf("create %s: %v", file, err)
		}
	}

	names, err := svc.ListStored()
	if err != nil {
		t.Fatalf("ListStored failed: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected 1 JSON file, got %d", len(names))
	}
	if names[0] != "work" {
		t.Errorf("expected 'work', got %q", names[0])
	}
}

func TestListStored_IgnoresDirectories(t *testing.T) {
	svc, fs := newTestService(t)

	if err := fs.MkdirAll("/store/subdir", 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := afero.WriteFile(fs, "/store/work.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	names, err := svc.ListStored()
	if err != nil {
		t.Fatalf("ListStored failed: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected 1 file (ignoring directory), got %d", len(names))
	}
}

func TestListEntries_ActiveUnmodified(t *testing.T) {
	svc, fs := newTestService(t)

	// Create stored setting
	if err := afero.WriteFile(fs, "/store/work.json", []byte("content"), 0o644); err != nil {
		t.Fatalf("setup stored: %v", err)
	}

	// Set as active
	if err := svc.SetActiveName("work"); err != nil {
		t.Fatalf("set active: %v", err)
	}

	// Create active file with same content
	activePath := "/active/settings.json"
	if err := afero.WriteFile(fs, activePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("setup active: %v", err)
	}

	mockHash := func(path string) (string, error) {
		return "samehash", nil
	}

	entries, err := svc.ListEntries(activePath, mockHash)
	if err != nil {
		t.Fatalf("ListEntries failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Name != "work" {
		t.Errorf("expected name 'work', got %q", entry.Name)
	}
	if entry.Prefix != "*" {
		t.Errorf("expected prefix '*', got %q", entry.Prefix)
	}
	if !contains(entry.Qualifiers, "active") {
		t.Error("should have 'active' qualifier")
	}
	if contains(entry.Qualifiers, "modified") {
		t.Error("should not have 'modified' qualifier")
	}
}

func TestListEntries_ActiveModified(t *testing.T) {
	svc, fs := newTestService(t)

	if err := afero.WriteFile(fs, "/store/work.json", []byte("original"), 0o644); err != nil {
		t.Fatalf("setup stored: %v", err)
	}
	if err := svc.SetActiveName("work"); err != nil {
		t.Fatalf("set active: %v", err)
	}

	activePath := "/active/settings.json"
	if err := afero.WriteFile(fs, activePath, []byte("modified"), 0o644); err != nil {
		t.Fatalf("setup active: %v", err)
	}

	hashCalls := 0
	mockHash := func(path string) (string, error) {
		hashCalls++
		if path == activePath {
			return "hash1", nil
		}
		return "hash2", nil
	}

	entries, err := svc.ListEntries(activePath, mockHash)
	if err != nil {
		t.Fatalf("ListEntries failed: %v", err)
	}

	entry := entries[0]
	if !contains(entry.Qualifiers, "modified") {
		t.Error("should have 'modified' qualifier")
	}
}

func TestListEntries_MissingActive(t *testing.T) {
	svc, fs := newTestService(t)

	if err := afero.WriteFile(fs, "/store/work.json", []byte("data"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := svc.SetActiveName("ghost"); err != nil {
		t.Fatalf("set active: %v", err)
	}

	mockHash := func(path string) (string, error) {
		return "hash", nil
	}

	entries, err := svc.ListEntries("/active/settings.json", mockHash)
	if err != nil {
		t.Fatalf("ListEntries failed: %v", err)
	}

	var ghostEntry *ListEntry
	for i := range entries {
		if entries[i].Name == "ghost" {
			ghostEntry = &entries[i]
			break
		}
	}

	if ghostEntry == nil {
		t.Fatal("expected entry for missing active setting")
	}
	if ghostEntry.Prefix != "!" {
		t.Errorf("expected prefix '!', got %q", ghostEntry.Prefix)
	}
	if !contains(ghostEntry.Qualifiers, "missing!") {
		t.Error("should have 'missing!' qualifier")
	}
}

func TestListEntries_UnsavedCurrent(t *testing.T) {
	svc, fs := newTestService(t)

	// No active name set
	activePath := "/active/settings.json"
	if err := afero.WriteFile(fs, activePath, []byte("unsaved"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	mockHash := func(path string) (string, error) {
		if path == activePath {
			return "somehash", nil
		}
		return "", nil
	}

	entries, err := svc.ListEntries(activePath, mockHash)
	if err != nil {
		t.Fatalf("ListEntries failed: %v", err)
	}

	var unsavedEntry *ListEntry
	for i := range entries {
		if entries[i].Plain {
			unsavedEntry = &entries[i]
			break
		}
	}

	if unsavedEntry == nil {
		t.Fatal("expected unsaved entry")
	}
	if unsavedEntry.Prefix != "*" {
		t.Errorf("expected prefix '*', got %q", unsavedEntry.Prefix)
	}
	if !unsavedEntry.Plain {
		t.Error("should be marked as plain")
	}
}

func TestListEntries_InactiveSettings(t *testing.T) {
	svc, fs := newTestService(t)

	if err := afero.WriteFile(fs, "/store/work.json", []byte("data1"), 0o644); err != nil {
		t.Fatalf("setup work: %v", err)
	}
	if err := afero.WriteFile(fs, "/store/personal.json", []byte("data2"), 0o644); err != nil {
		t.Fatalf("setup personal: %v", err)
	}
	if err := svc.SetActiveName("work"); err != nil {
		t.Fatalf("set active: %v", err)
	}

	mockHash := func(path string) (string, error) {
		return "hash", nil
	}

	entries, err := svc.ListEntries("/active/settings.json", mockHash)
	if err != nil {
		t.Fatalf("ListEntries failed: %v", err)
	}

	var personalEntry *ListEntry
	for i := range entries {
		if entries[i].Name == "personal" {
			personalEntry = &entries[i]
			break
		}
	}

	if personalEntry == nil {
		t.Fatal("expected entry for personal")
	}
	if personalEntry.Prefix != " " {
		t.Errorf("expected prefix ' ', got %q", personalEntry.Prefix)
	}
	if contains(personalEntry.Qualifiers, "active") {
		t.Error("should not have 'active' qualifier")
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
