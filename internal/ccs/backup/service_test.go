package backup

// Tests for content-addressed backup with SHA-256 deduplication.
//
// Focus: CalculateHash (SHA-256, empty file handling), BackupFile (deduplication),
// PruneBackups (time-based cleanup).

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/storage"
	"github.com/spf13/afero"
)

func newTestService(t *testing.T) (*Service, afero.Fs) {
	t.Helper()
	fs := afero.NewMemMapFs()
	stor := storage.New(fs)
	backupDir := "/backups"
	if err := fs.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("setup backup dir: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := New(stor, backupDir, logger)
	return svc, fs
}

func TestCalculateHash_RegularFile(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/file.json"
	if err := afero.WriteFile(fs, path, []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	hash, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("CalculateHash failed: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if len(hash) != 64 {
		t.Errorf("expected SHA-256 hash (64 chars), got %d chars", len(hash))
	}
}

func TestCalculateHash_EmptyFile(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/empty.json"
	if err := afero.WriteFile(fs, path, []byte{}, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	hash, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("CalculateHash failed: %v", err)
	}
	if hash != "empty" {
		t.Errorf("expected 'empty' for empty file, got %q", hash)
	}
}

func TestCalculateHash_MissingFile(t *testing.T) {
	svc, _ := newTestService(t)

	hash, err := svc.CalculateHash("/nonexistent")
	if err != nil {
		t.Fatalf("CalculateHash should not error for missing file: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash for missing file, got %q", hash)
	}
}

func TestCalculateHash_Deterministic(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/file.json"
	content := []byte("deterministic content")
	if err := afero.WriteFile(fs, path, content, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	hash1, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}

	hash2, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}

	if hash1 != hash2 {
		t.Error("hash should be deterministic")
	}
}

func TestBackupFile_CreatesNewBackup(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/file.json"
	if err := afero.WriteFile(fs, path, []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := svc.BackupFile(path); err != nil {
		t.Fatalf("BackupFile failed: %v", err)
	}

	// Verify backup was created
	entries, err := afero.ReadDir(fs, svc.BackupDir())
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 backup, got %d", len(entries))
	}
	name := entries[0].Name()
	if len(name) < 5 || name[len(name)-5:] != ".json" {
		t.Errorf("backup should have .json extension, got %q", name)
	}
}

func TestBackupFile_DeduplicationCreatesOnlyOneFile(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/file.json"
	if err := afero.WriteFile(fs, path, []byte("same content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// First backup
	if err := svc.BackupFile(path); err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// Second backup with identical content (should deduplicate)
	if err := svc.BackupFile(path); err != nil {
		t.Fatalf("second backup: %v", err)
	}

	// Should only have one backup file (content-addressed deduplication)
	entries, err := afero.ReadDir(fs, svc.BackupDir())
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 backup (deduplicated), got %d", len(entries))
	}

	// Verify the backup contains the correct content
	hash, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("calculate hash: %v", err)
	}
	backupPath := filepath.Join(svc.BackupDir(), hash+".json")
	content, err := afero.ReadFile(fs, backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(content) != "same content" {
		t.Errorf("backup content mismatch: got %q", string(content))
	}
}

func TestBackupFile_DifferentContentCreatesSeparateBackups(t *testing.T) {
	svc, fs := newTestService(t)

	// First file
	path1 := "/test/file1.json"
	if err := afero.WriteFile(fs, path1, []byte("content A"), 0o644); err != nil {
		t.Fatalf("setup file1: %v", err)
	}
	if err := svc.BackupFile(path1); err != nil {
		t.Fatalf("backup file1: %v", err)
	}

	// Second file with different content
	path2 := "/test/file2.json"
	if err := afero.WriteFile(fs, path2, []byte("content B"), 0o644); err != nil {
		t.Fatalf("setup file2: %v", err)
	}
	if err := svc.BackupFile(path2); err != nil {
		t.Fatalf("backup file2: %v", err)
	}

	// Should have two separate backups
	entries, err := afero.ReadDir(fs, svc.BackupDir())
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 backups, got %d", len(entries))
	}
}

func TestBackupFile_MissingFile(t *testing.T) {
	svc, fs := newTestService(t)

	err := svc.BackupFile("/nonexistent")
	if err != nil {
		t.Fatalf("BackupFile should not error for missing file: %v", err)
	}

	// No backup should be created
	entries, err := afero.ReadDir(fs, svc.BackupDir())
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 backups for missing file, got %d", len(entries))
	}
}

func TestBackupFile_EmptyFile(t *testing.T) {
	svc, fs := newTestService(t)

	path := "/test/empty.json"
	if err := afero.WriteFile(fs, path, []byte{}, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := svc.BackupFile(path); err != nil {
		t.Fatalf("BackupFile failed: %v", err)
	}

	// Should create backup with name "empty.json"
	backupPath := filepath.Join(svc.BackupDir(), "empty.json")
	exists, err := afero.Exists(fs, backupPath)
	if err != nil {
		t.Fatalf("check backup: %v", err)
	}
	if !exists {
		t.Error("backup for empty file should exist")
	}
}

func TestBackupFile_PreservesContent(t *testing.T) {
	svc, fs := newTestService(t)

	originalContent := []byte(`{"setting": "value"}`)
	path := "/test/file.json"
	if err := afero.WriteFile(fs, path, originalContent, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := svc.BackupFile(path); err != nil {
		t.Fatalf("BackupFile failed: %v", err)
	}

	hash, err := svc.CalculateHash(path)
	if err != nil {
		t.Fatalf("calculate hash: %v", err)
	}
	backupPath := filepath.Join(svc.BackupDir(), hash+".json")

	backupContent, err := afero.ReadFile(fs, backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}

	if string(backupContent) != string(originalContent) {
		t.Errorf("backup content mismatch: got %q, want %q", string(backupContent), string(originalContent))
	}
}

func TestPruneBackups_DeletesOldFiles(t *testing.T) {
	svc, fs := newTestService(t)

	time1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time1.Add(48 * time.Hour)

	// Create old backup
	oldPath := filepath.Join(svc.BackupDir(), "old.json")
	if err := afero.WriteFile(fs, oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("create old backup: %v", err)
	}
	if err := fs.Chtimes(oldPath, time1, time1); err != nil {
		t.Fatalf("set old time: %v", err)
	}

	// Create recent backup
	recentPath := filepath.Join(svc.BackupDir(), "recent.json")
	if err := afero.WriteFile(fs, recentPath, []byte("recent"), 0o644); err != nil {
		t.Fatalf("create recent backup: %v", err)
	}
	if err := fs.Chtimes(recentPath, time2, time2); err != nil {
		t.Fatalf("set recent time: %v", err)
	}

	// Prune from time2 + 1 hour, looking back 24 hours
	svc.SetNow(func() time.Time { return time2.Add(1 * time.Hour) })
	deleted, err := svc.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Old should be gone
	exists, _ := afero.Exists(fs, oldPath)
	if exists {
		t.Error("old backup should be deleted")
	}

	// Recent should remain
	exists, _ = afero.Exists(fs, recentPath)
	if !exists {
		t.Error("recent backup should remain")
	}
}

func TestPruneBackups_IgnoresDirectories(t *testing.T) {
	svc, fs := newTestService(t)

	dirPath := filepath.Join(svc.BackupDir(), "subdir")
	if err := fs.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	deleted, err := svc.PruneBackups(0)
	if err != nil {
		t.Fatalf("PruneBackups failed: %v", err)
	}
	if deleted != 0 {
		t.Error("should not delete directories")
	}

	// Directory should still exist
	exists, _ := afero.DirExists(fs, dirPath)
	if !exists {
		t.Error("directory should not be deleted")
	}
}

func TestPruneBackups_NoFilesToDelete(t *testing.T) {
	svc, fs := newTestService(t)

	// Create recent backup
	path := filepath.Join(svc.BackupDir(), "recent.json")
	if err := afero.WriteFile(fs, path, []byte("data"), 0o644); err != nil {
		t.Fatalf("create backup: %v", err)
	}

	svc.SetNow(func() time.Time { return time.Now() })
	deleted, err := svc.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestPruneBackups_EmptyDirectory(t *testing.T) {
	svc, _ := newTestService(t)

	deleted, err := svc.PruneBackups(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneBackups failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted from empty directory, got %d", deleted)
	}
}

func TestPruneBackups_ErrorOnNonExistentDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	stor := storage.New(fs)
	svc := New(stor, "/nonexistent", nil)

	_, err := svc.PruneBackups(24 * time.Hour)
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist in chain, got: %v", err)
	}
}
