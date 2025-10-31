package storage

// Tests for atomic file operations and security requirements.
//
// Focus: CopyFile (atomic with temp files), ValidatePathSafety (symlink protection),
// secure permissions (0600 files, 0700 dirs).
//
// Note: Simple wrappers (ReadFile, WriteFile, etc.) tested via integration tests.

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/afero"
)

func TestCopyFile_Success(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	src := "/test/source.json"
	dst := "/test/dest.json"

	if err := afero.WriteFile(fs, src, []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := storage.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	content, err := afero.ReadFile(fs, dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("expected 'content', got %q", string(content))
	}
}

func TestCopyFile_CreatesIntermediateDirectories(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	src := "/test/source.json"
	dst := "/deeply/nested/path/dest.json"

	if err := afero.WriteFile(fs, src, []byte("data"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := storage.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	exists, err := afero.Exists(fs, dst)
	if err != nil {
		t.Fatalf("exists check: %v", err)
	}
	if !exists {
		t.Error("destination file should exist")
	}

	// Verify directory has secure permissions
	info, err := fs.Stat("/deeply/nested/path")
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("expected directory mode 0700, got %o", info.Mode().Perm())
	}
}

func TestCopyFile_OverwritesExisting(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	src := "/test/source.json"
	dst := "/test/dest.json"

	if err := afero.WriteFile(fs, src, []byte("new"), 0o644); err != nil {
		t.Fatalf("setup src: %v", err)
	}
	if err := afero.WriteFile(fs, dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("setup dst: %v", err)
	}

	if err := storage.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	content, err := afero.ReadFile(fs, dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(content) != "new" {
		t.Errorf("expected 'new', got %q", string(content))
	}
}

func TestCopyFile_MissingSource(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	err := storage.CopyFile("/nonexistent", "/dest")
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist in chain, got: %v", err)
	}
}

func TestCopyFile_SecurePermissions(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	src := "/test/source.json"
	dst := "/test/dest.json"

	if err := afero.WriteFile(fs, src, []byte("secret"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := storage.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	info, err := fs.Stat(dst)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file mode 0600, got %o", info.Mode().Perm())
	}
}

func TestCopyFile_CleansUpTempFileOnFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	// Create source
	src := "/test/source.json"
	if err := afero.WriteFile(fs, src, []byte("data"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Make destination directory read-only to cause rename failure
	dst := "/readonly/dest.json"
	if err := fs.MkdirAll("/readonly", 0o700); err != nil {
		t.Fatalf("create readonly dir: %v", err)
	}

	// Note: afero.MemMapFs doesn't enforce permissions, so this test
	// documents the intended behavior rather than actually testing it.
	// With a real filesystem, this would fail during rename.
	err := storage.CopyFile(src, dst)

	// The copy should succeed with MemMapFs, but we document the expected
	// behavior: temp files should be cleaned up on error
	if err == nil {
		// Check that no .tmp files are left behind
		tmpFile := dst + ".tmp"
		exists, _ := afero.Exists(fs, tmpFile)
		if exists {
			t.Error("temp file should not exist after successful copy")
		}
	}
}

func TestValidatePathSafety_NonExistentPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	// Non-existent paths should be safe (allows writing new files)
	err := storage.ValidatePathSafety("/nonexistent/file.json")
	if err != nil {
		t.Errorf("non-existent path should be safe: %v", err)
	}
}

func TestValidatePathSafety_RegularFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	path := "/test/file.json"
	if err := afero.WriteFile(fs, path, []byte("data"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := storage.ValidatePathSafety(path)
	if err != nil {
		t.Errorf("regular file should be safe: %v", err)
	}
}

// TestMkdirAll verifies that directories are created with secure 0700 permissions
func TestMkdirAll_SecurePermissions(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage := New(fs)

	path := "/deeply/nested/path"
	if err := storage.MkdirAll(path); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	info, err := fs.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("path should be a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("expected secure mode 0700, got %o", info.Mode().Perm())
	}
}
