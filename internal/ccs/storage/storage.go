package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// Storage provides low-level file operations with security validations.
type Storage struct {
	fs afero.Fs
}

// New creates a new Storage instance.
func New(fs afero.Fs) *Storage {
	return &Storage{fs: fs}
}

// FileSystem returns the underlying filesystem.
func (s *Storage) FileSystem() afero.Fs {
	return s.fs
}

// ValidatePathSafety checks that the path is not a symlink, preventing symlink attacks.
// It returns nil if the path doesn't exist or is a regular file/directory.
func (s *Storage) ValidatePathSafety(path string) error {
	// Try to use Lstat if the filesystem supports it
	if lstater, ok := s.fs.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil // Non-existent paths are safe to write to
			}
			return fmt.Errorf("failed to check path: %w", err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to operate on symlink: %s", path)
		}
	}
	// If Lstat not available, fall through (in-memory filesystems don't support symlinks anyway)
	return nil
}

// CopyFile copies a file from src to dst, atomically replacing the destination.
func (s *Storage) CopyFile(src, dst string) (err error) {
	// Validate that paths are not symlinks
	if err := s.ValidatePathSafety(src); err != nil {
		return fmt.Errorf("validate source: %w", err)
	}
	if err := s.ValidatePathSafety(dst); err != nil {
		return fmt.Errorf("validate destination: %w", err)
	}

	source, err := s.fs.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() {
		if cerr := source.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close source: %w", cerr)
		}
	}()

	dir := filepath.Dir(dst)
	if err := s.fs.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create temp file in same directory (enables atomic rename)
	tmp := dst + ".tmp"
	dest, err := s.fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	// Copy with proper error handling
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()

	if copyErr != nil || closeErr != nil {
		s.fs.Remove(tmp)
		if copyErr != nil {
			return fmt.Errorf("copy data: %w", copyErr)
		}
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	// Atomic rename: Unix rename() atomically replaces the destination
	if err := s.fs.Rename(tmp, dst); err != nil {
		s.fs.Remove(tmp)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// ReadFile reads the entire file.
func (s *Storage) ReadFile(path string) ([]byte, error) {
	return afero.ReadFile(s.fs, path)
}

// WriteFile writes data to a file with secure permissions.
func (s *Storage) WriteFile(path string, data []byte) error {
	return afero.WriteFile(s.fs, path, data, 0o600)
}

// Exists checks if a path exists.
func (s *Storage) Exists(path string) (bool, error) {
	return afero.Exists(s.fs, path)
}

// Stat returns file information.
func (s *Storage) Stat(path string) (os.FileInfo, error) {
	return s.fs.Stat(path)
}

// MkdirAll creates directory with secure permissions.
func (s *Storage) MkdirAll(path string) error {
	return s.fs.MkdirAll(path, 0o700)
}

// ReadDir reads directory contents.
func (s *Storage) ReadDir(path string) ([]os.FileInfo, error) {
	return afero.ReadDir(s.fs, path)
}

// Remove deletes a file.
func (s *Storage) Remove(path string) error {
	return s.fs.Remove(path)
}

// Chtimes changes file access and modification times.
func (s *Storage) Chtimes(path string, atime, mtime time.Time) error {
	return s.fs.Chtimes(path, atime, mtime)
}
