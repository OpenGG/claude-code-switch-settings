package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/storage"
)

// Service handles backup operations with content-addressed storage.
type Service struct {
	storage   *storage.Storage
	backupDir string
	now       func() time.Time
	logger    *slog.Logger
}

// New creates a new backup Service.
func New(storage *storage.Storage, backupDir string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		storage:   storage,
		backupDir: backupDir,
		now:       time.Now,
		logger:    logger,
	}
}

// SetNow allows overriding the clock for testing.
func (s *Service) SetNow(now func() time.Time) {
	if now == nil {
		s.now = time.Now
		return
	}
	s.now = now
}

// CalculateHash returns the SHA-256 hash of the given file.
// Empty files return a special "empty" marker and log a warning.
// Missing files return an empty string without error.
func (s *Service) CalculateHash(path string) (string, error) {
	// Validate path safety before accessing to prevent symlink attacks
	if err := s.storage.ValidatePathSafety(path); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	info, err := s.storage.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("failed to stat file for hashing: %w", err)
	}
	if info.Size() == 0 {
		s.logger.Warn("empty file detected during hash calculation",
			"path", path,
			"operation", "hash")
		return "empty", nil
	}

	f, err := s.storage.FileSystem().Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BackupFile creates a content-addressed backup of the file at path.
//
// The backup uses SHA-256 hash as filename, enabling deduplication:
//   - Identical content reuses the same backup file
//   - Modified time (mtime) is updated on each backup event
//   - Empty files are backed up with hash "empty" (with warning logged)
//   - Missing files are silently skipped
//
// Backup files are stored in the backup directory as:
//
//	<sha256-hash>.json or empty.json
//
// This approach ensures:
//   - Multiple backups of identical content don't waste space
//   - The prune command can use mtime to determine backup age
//   - Each unique settings version is preserved exactly once
func (s *Service) BackupFile(path string) (err error) {
	// Note: CalculateHash already validates path safety via ValidatePathSafety
	hash, err := s.CalculateHash(path)
	if err != nil {
		return err
	}
	if hash == "" {
		// File doesn't exist - nothing to backup
		return nil
	}

	source, err := s.storage.FileSystem().Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to open file for backup: %w", err)
	}
	defer func() {
		if cerr := source.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close source: %w", cerr)
		}
	}()

	backupPath := filepath.Join(s.backupDir, hash+".json")
	now := s.now()
	if _, err := s.storage.Stat(backupPath); err == nil {
		// Backup already exists - just update timestamp for deduplication
		if err := s.storage.Chtimes(backupPath, now, now); err != nil {
			return fmt.Errorf("failed to update backup timestamp: %w", err)
		}
		s.logger.Debug("backup already exists, updated timestamp",
			"path", path,
			"hash", hash,
			"backup_path", backupPath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat backup: %w", err)
	}

	dst, err := s.storage.FileSystem().OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	_, copyErr := io.Copy(dst, source)
	closeErr := dst.Close()

	if copyErr != nil {
		s.storage.Remove(backupPath)
		return fmt.Errorf("failed to copy backup: %w", copyErr)
	}
	if closeErr != nil {
		s.storage.Remove(backupPath)
		return fmt.Errorf("failed to close backup: %w", closeErr)
	}

	if err := s.storage.Chtimes(backupPath, now, now); err != nil {
		return fmt.Errorf("failed to update backup timestamp: %w", err)
	}

	s.logger.Info("backup created",
		"path", path,
		"hash", hash,
		"backup_path", backupPath)

	return nil
}

// PruneBackups removes backup files older than the specified duration.
//
// The function uses modification time (mtime) to determine backup age. Since
// content-addressed backups update mtime on each backup event, this effectively
// prunes backups that haven't been referenced recently.
//
// Returns the number of backups deleted and any error encountered.
func (s *Service) PruneBackups(olderThan time.Duration) (int, error) {
	entries, err := s.storage.ReadDir(s.backupDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read backup directory: %w", err)
	}
	cutoff := s.now().Add(-olderThan)
	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(s.backupDir, entry.Name())
		info, err := s.storage.Stat(path)
		if err != nil {
			return deleted, fmt.Errorf("failed to stat backup: %w", err)
		}
		if info.ModTime().Before(cutoff) {
			if err := s.storage.Remove(path); err != nil {
				return deleted, fmt.Errorf("failed to delete backup: %w", err)
			}
			deleted++
		}
	}
	return deleted, nil
}

// BackupDir returns the backup directory path.
func (s *Service) BackupDir() string {
	return s.backupDir
}
