package ccs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// Exported error variables allow callers to use errors.Is() for error checking.
var (
	ErrSettingsNameEmpty        = errors.New("settings name cannot be empty")
	ErrSettingsNameDot          = errors.New("settings name cannot be '.' or '..'")
	ErrSettingsNameNonPrintable = errors.New("settings name contains non-printable characters")
	ErrSettingsNameInvalidChars = errors.New("settings name contains invalid characters (<>:\"/|?*)")
	ErrSettingsNameReserved     = errors.New("settings name is a reserved system filename")
	ErrSettingsNameNullByte     = errors.New("settings name contains null byte")
)

var reservedNamePattern = regexp.MustCompile(`^(?i)(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)
var invalidCharsPattern = regexp.MustCompile(`[<>:"/\\|?*]`)

// Manager coordinates settings operations using an injected filesystem and clock.
// It provides atomic file operations, content-addressed backups, and comprehensive
// validation of settings names to prevent security issues like path traversal and
// symlink attacks.
type Manager struct {
	fs      afero.Fs
	homeDir string
	now     func() time.Time
	logger  *slog.Logger
}

// NewManager constructs a Manager using the provided filesystem and home directory.
// If logger is nil, a default logger will be created that discards all output.
func NewManager(fs afero.Fs, homeDir string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Manager{
		fs:      fs,
		homeDir: homeDir,
		now:     time.Now,
		logger:  logger,
	}
}

// InitInfra ensures that required directories exist.
func (m *Manager) InitInfra() error {
	paths := []string{m.claudeDir(), m.settingsStoreDir(), m.backupDir()}
	for _, p := range paths {
		if err := m.fs.MkdirAll(p, 0o700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", p, err)
		}
	}
	return nil
}

// CalculateHash returns the SHA-256 hash of the given file.
// Empty files return a special "empty" marker and log a warning.
// Missing files return an empty string without error.
func (m *Manager) CalculateHash(path string) (string, error) {
	info, err := m.fs.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("failed to stat file for hashing: %w", err)
	}
	if info.Size() == 0 {
		m.logger.Warn("empty file detected during hash calculation",
			"path", path,
			"operation", "hash")
		return "empty", nil
	}

	f, err := m.fs.Open(path)
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

// backupFile creates a content-addressed backup of the file at path.
//
// The backup uses SHA-256 hash as filename, enabling deduplication:
//   - Identical content reuses the same backup file
//   - Modified time (mtime) is updated on each backup event
//   - Empty files are backed up with hash "empty" (with warning logged)
//   - Missing files are silently skipped
//
// Backup files are stored in ~/.claude/switch-settings-backup/ as:
//
//	<sha256-hash>.json or empty.json
//
// This approach ensures:
//   - Multiple backups of identical content don't waste space
//   - The prune command can use mtime to determine backup age
//   - Each unique settings version is preserved exactly once
func (m *Manager) backupFile(path string) (err error) {
	hash, err := m.CalculateHash(path)
	if err != nil {
		return err
	}
	if hash == "" {
		// File doesn't exist - nothing to backup
		return nil
	}

	source, err := m.fs.Open(path)
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

	backupPath := filepath.Join(m.backupDir(), hash+".json")
	now := m.now()
	if _, err := m.fs.Stat(backupPath); err == nil {
		// Backup already exists - just update timestamp for deduplication
		if err := m.fs.Chtimes(backupPath, now, now); err != nil {
			return fmt.Errorf("failed to update backup timestamp: %w", err)
		}
		m.logger.Debug("backup already exists, updated timestamp",
			"path", path,
			"hash", hash,
			"backup_path", backupPath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat backup: %w", err)
	}

	dst, err := m.fs.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	_, copyErr := io.Copy(dst, source)
	closeErr := dst.Close()

	if copyErr != nil {
		m.fs.Remove(backupPath)
		return fmt.Errorf("failed to copy backup: %w", copyErr)
	}
	if closeErr != nil {
		m.fs.Remove(backupPath)
		return fmt.Errorf("failed to close backup: %w", closeErr)
	}

	if err := m.fs.Chtimes(backupPath, now, now); err != nil {
		return fmt.Errorf("failed to update backup timestamp: %w", err)
	}

	m.logger.Info("backup created",
		"path", path,
		"hash", hash,
		"backup_path", backupPath)

	return nil
}

// GetActiveSettingsName returns the currently active settings name.
func (m *Manager) GetActiveSettingsName() string {
	content, err := afero.ReadFile(m.fs, m.activeStatePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// SetActiveSettings sets the active settings name.
func (m *Manager) SetActiveSettings(name string) error {
	return afero.WriteFile(m.fs, m.activeStatePath(), []byte(name), 0o600)
}

// ValidateSettingsName validates the provided settings name for security and compatibility.
//
// The function checks for:
//   - Empty names or whitespace-only names
//   - Dot navigation (. or ..)
//   - Null bytes (path traversal attack vector)
//   - Non-printable ASCII characters
//   - Invalid filesystem characters (<>:"/\|?*)
//   - Reserved Windows filenames (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
//
// Returns (true, nil) if valid, or (false, error) with a descriptive error.
func (m *Manager) ValidateSettingsName(name string) (bool, error) {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return false, ErrSettingsNameEmpty
	}
	if trimmed == "." || trimmed == ".." {
		return false, ErrSettingsNameDot
	}

	// Explicit null byte check for defense-in-depth
	if strings.ContainsRune(trimmed, 0) {
		return false, ErrSettingsNameNullByte
	}

	for _, r := range trimmed {
		if r < 0x20 || r > 0x7e {
			return false, ErrSettingsNameNonPrintable
		}
		if r == 0x7f {
			return false, ErrSettingsNameNonPrintable
		}
	}
	if invalidCharsPattern.MatchString(trimmed) {
		return false, ErrSettingsNameInvalidChars
	}
	if reservedNamePattern.MatchString(trimmed) {
		return false, ErrSettingsNameReserved
	}
	return true, nil
}

func (m *Manager) normalizeSettingsName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if ok, err := m.ValidateSettingsName(trimmed); !ok {
		if err != nil {
			return "", fmt.Errorf("invalid settings name: %w", err)
		}
		return "", errors.New("invalid settings name")
	}
	return trimmed, nil
}

// validatePathSafety checks that the path is not a symlink, preventing symlink attacks.
// It returns nil if the path doesn't exist or is a regular file/directory.
func (m *Manager) validatePathSafety(path string) error {
	// Try to use Lstat if the filesystem supports it
	if lstater, ok := m.fs.(afero.Lstater); ok {
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

// copyFile copies a file from src to dst, atomically replacing the destination.
func (m *Manager) copyFile(src, dst string) (err error) {
	// Validate that paths are not symlinks
	if err := m.validatePathSafety(src); err != nil {
		return fmt.Errorf("validate source: %w", err)
	}
	if err := m.validatePathSafety(dst); err != nil {
		return fmt.Errorf("validate destination: %w", err)
	}

	source, err := m.fs.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() {
		if cerr := source.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close source: %w", cerr)
		}
	}()

	dir := filepath.Dir(dst)
	if err := m.fs.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	tmp := dst + ".tmp"
	dest, err := m.fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()

	if copyErr != nil {
		m.fs.Remove(tmp)
		return fmt.Errorf("copy data: %w", copyErr)
	}
	if closeErr != nil {
		m.fs.Remove(tmp)
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	// Atomic rename: Unix rename() atomically replaces the destination
	if err := m.fs.Rename(tmp, dst); err != nil {
		m.fs.Remove(tmp)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// Use activates the specified settings profile by copying it to the active settings location.
//
// The operation performs the following steps atomically:
//  1. Validates the profile name (see ValidateSettingsName)
//  2. Verifies the profile exists in the settings store
//  3. Backs up the current active settings (if any)
//  4. Atomically copies the profile to ~/.claude/settings.json
//  5. Updates the active state file to track the current profile
//
// The operation is atomic - if it fails at any step, the current settings remain unchanged.
//
// Returns an error if:
//   - The profile name is invalid (see ValidateSettingsName)
//   - The profile doesn't exist in the settings store
//   - File operations fail (permissions, disk space, etc.)
//
// Example:
//
//	err := mgr.Use("work")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (m *Manager) Use(name string) error {
	if err := m.InitInfra(); err != nil {
		return err
	}
	normalized, err := m.normalizeSettingsName(name)
	if err != nil {
		return err
	}
	targetPath := m.storedSettingsPath(normalized)
	if exists, err := afero.Exists(m.fs, targetPath); err != nil {
		return fmt.Errorf("failed to inspect target settings: %w", err)
	} else if !exists {
		return fmt.Errorf("settings '%s' not found", normalized)
	}
	if err := m.backupFile(m.activeSettingsPath()); err != nil {
		return err
	}
	if err := m.copyFile(targetPath, m.activeSettingsPath()); err != nil {
		return fmt.Errorf("failed to copy settings: %w", err)
	}
	if err := m.SetActiveSettings(normalized); err != nil {
		return fmt.Errorf("failed to update active settings: %w", err)
	}
	return nil
}

// Save persists the current active settings to a named profile in the settings store.
//
// The operation performs the following steps atomically:
//  1. Validates the target profile name (see ValidateSettingsName)
//  2. Verifies that ~/.claude/settings.json exists
//  3. Backs up the existing profile (if overwriting)
//  4. Atomically copies current settings to the profile location
//  5. Updates the active state to track this profile
//
// The operation is atomic - if it fails at any step, existing profiles remain unchanged.
//
// Returns an error if:
//   - The active settings.json doesn't exist
//   - The target profile name is invalid
//   - File operations fail (permissions, disk space, etc.)
//
// Example:
//
//	err := mgr.Save("work-settings")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (m *Manager) Save(targetName string) error {
	if err := m.InitInfra(); err != nil {
		return err
	}
	activePath := m.activeSettingsPath()
	if exists, err := afero.Exists(m.fs, activePath); err != nil {
		return fmt.Errorf("failed to inspect settings.json: %w", err)
	} else if !exists {
		return errors.New("settings.json not found. Nothing to save.")
	}
	normalized, err := m.normalizeSettingsName(targetName)
	if err != nil {
		return err
	}
	targetPath := m.storedSettingsPath(normalized)
	if err := m.backupFile(targetPath); err != nil {
		return err
	}
	if err := m.copyFile(activePath, targetPath); err != nil {
		return fmt.Errorf("failed to store settings: %w", err)
	}
	if err := m.SetActiveSettings(normalized); err != nil {
		return fmt.Errorf("failed to update active settings: %w", err)
	}
	return nil
}

// StoredSettings returns the names of all stored settings profiles, sorted lexicographically.
//
// The function scans the settings store directory (~/.claude/switch-settings/) and returns
// only the base names (without .json extension) of regular files.
//
// Returns an error if the settings store directory cannot be read.
func (m *Manager) StoredSettings() ([]string, error) {
	if err := m.InitInfra(); err != nil {
		return nil, err
	}
	dir := m.settingsStoreDir()
	entries, err := afero.ReadDir(m.fs, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read settings store: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// ListEntries describes each available settings entry for list output.
type ListEntry struct {
	Name       string
	Prefix     string
	Qualifiers []string
	Plain      bool
}

// ListSettings computes formatted entries for display in the list command.
//
// Each entry includes:
//   - Name: The profile name
//   - Prefix: Visual indicator (* = active, ! = missing, space = inactive)
//   - Qualifiers: Tags like "active", "modified", "missing!"
//   - Plain: Whether to skip bracket formatting
//
// The function compares stored profiles with the active settings and annotates
// entries with their status. If the active profile has been modified locally,
// it will be marked with "modified".
//
// Returns an error if the settings store or active settings cannot be accessed.
func (m *Manager) ListSettings() ([]ListEntry, error) {
	if err := m.InitInfra(); err != nil {
		return nil, err
	}
	activeName := m.GetActiveSettingsName()
	currentHash, err := m.CalculateHash(m.activeSettingsPath())
	if err != nil {
		return nil, err
	}
	names, err := m.StoredSettings()
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	activeHandled := false
	for _, name := range names {
		entry := ListEntry{Name: name, Prefix: " "}
		if name == activeName {
			entry.Prefix = "*"
			activeHandled = true
			storedHash, err := m.CalculateHash(m.storedSettingsPath(name))
			if err != nil {
				return nil, err
			}
			entry.Qualifiers = append(entry.Qualifiers, "active")
			if currentHash != "" && storedHash != "" && currentHash != storedHash {
				entry.Qualifiers = append(entry.Qualifiers, "modified")
			}
		} else {
			entry.Prefix = " "
		}
		entries = append(entries, entry)
	}

	if activeName != "" && !activeHandled {
		entries = append(entries, ListEntry{
			Name:       activeName,
			Prefix:     "!",
			Qualifiers: []string{"active", "missing!"},
		})
	} else if activeName == "" && currentHash != "" {
		entries = append(entries, ListEntry{
			Name:   "(Current settings.json is unsaved)",
			Prefix: "*",
			Plain:  true,
		})
	}

	return entries, nil
}

// PruneBackups removes backup files older than the specified duration.
//
// The function uses modification time (mtime) to determine backup age. Since
// content-addressed backups update mtime on each backup event, this effectively
// prunes backups that haven't been referenced recently.
//
// Returns the number of backups deleted and any error encountered.
//
// Example:
//
//	// Delete backups older than 30 days
//	count, err := mgr.PruneBackups(30 * 24 * time.Hour)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Deleted %d backups\n", count)
func (m *Manager) PruneBackups(olderThan time.Duration) (int, error) {
	if err := m.InitInfra(); err != nil {
		return 0, err
	}
	dir := m.backupDir()
	entries, err := afero.ReadDir(m.fs, dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read backup directory: %w", err)
	}
	cutoff := m.now().Add(-olderThan)
	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := m.fs.Stat(path)
		if err != nil {
			return deleted, fmt.Errorf("failed to stat backup: %w", err)
		}
		if info.ModTime().Before(cutoff) {
			if err := m.fs.Remove(path); err != nil {
				return deleted, fmt.Errorf("failed to delete backup: %w", err)
			}
			deleted++
		}
	}
	return deleted, nil
}

// ActiveSettingsPath returns the path to settings.json for consumers like tests.
func (m *Manager) ActiveSettingsPath() string {
	return m.activeSettingsPath()
}

// ActiveStatePath returns the path to settings.json.active for consumers like tests.
func (m *Manager) ActiveStatePath() string {
	return m.activeStatePath()
}

// BackupDir returns the backup directory path.
func (m *Manager) BackupDir() string {
	return m.backupDir()
}

// SettingsStoreDir returns the store directory path.
func (m *Manager) SettingsStoreDir() string {
	return m.settingsStoreDir()
}

// FileSystem exposes the underlying filesystem.
func (m *Manager) FileSystem() afero.Fs {
	return m.fs
}

// StoredSettingsPath returns the full path to a stored settings file.
func (m *Manager) StoredSettingsPath(name string) (string, error) {
	normalized, err := m.normalizeSettingsName(name)
	if err != nil {
		return "", err
	}
	return m.storedSettingsPath(normalized), nil
}

// SetNow overrides the clock used by the manager.
func (m *Manager) SetNow(now func() time.Time) {
	if now == nil {
		m.now = time.Now
		return
	}
	m.now = now
}
