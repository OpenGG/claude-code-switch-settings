package ccs

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/afero"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/backup"
	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/domain"
	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/paths"
	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/settings"
	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/storage"
	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/validator"
)

// Re-export domain errors for backward compatibility
var (
	ErrSettingsNameEmpty        = domain.ErrSettingsNameEmpty
	ErrSettingsNameDot          = domain.ErrSettingsNameDot
	ErrSettingsNameNonPrintable = domain.ErrSettingsNameNonPrintable
	ErrSettingsNameInvalidChars = domain.ErrSettingsNameInvalidChars
	ErrSettingsNameReserved     = domain.ErrSettingsNameReserved
	ErrSettingsNameNullByte     = domain.ErrSettingsNameNullByte
)

// Manager coordinates settings operations using injected services.
// It provides atomic file operations, content-addressed backups, and comprehensive
// validation of settings names to prevent security issues like path traversal and
// symlink attacks.
//
// This is a thin orchestrator that delegates to specialized services:
//   - validator: Name validation and normalization
//   - storage: Low-level file operations with security checks
//   - backup: Content-addressed backup management
//   - settings: Settings persistence and retrieval
type Manager struct {
	paths *paths.PathBuilder

	// Services (dependency injection)
	validator *validator.Validator
	storage   *storage.Storage
	backup    *backup.Service
	settings  *settings.Service
}

// NewManager constructs a Manager using the provided filesystem and home directory.
// If logger is nil, a default logger will be created that discards all output.
func NewManager(fs afero.Fs, homeDir string, logger *slog.Logger) *Manager {
	// Create path builder
	pathBuilder := paths.New(homeDir)

	// Create storage layer
	stor := storage.New(fs)

	// Create backup service
	backupSvc := backup.New(stor, pathBuilder.BackupDir(), logger)

	// Create settings service
	settingsSvc := settings.New(stor, pathBuilder.SettingsStoreDir(), pathBuilder.ActiveStatePath())

	// Create validator
	val := validator.New()

	return &Manager{
		paths:     pathBuilder,
		validator: val,
		storage:   stor,
		backup:    backupSvc,
		settings:  settingsSvc,
	}
}

// InitInfra ensures that required directories exist.
func (m *Manager) InitInfra() error {
	dirs := []string{m.paths.ClaudeDir(), m.paths.SettingsStoreDir(), m.paths.BackupDir()}
	for _, p := range dirs {
		if err := m.storage.MkdirAll(p); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", p, err)
		}
	}
	return nil
}

// CalculateHash returns the SHA-256 hash of the given file.
// Empty files return a special "empty" marker and log a warning.
// Missing files return an empty string without error.
func (m *Manager) CalculateHash(path string) (string, error) {
	return m.backup.CalculateHash(path)
}

// GetActiveSettingsName returns the currently active settings name.
func (m *Manager) GetActiveSettingsName() string {
	return m.settings.GetActiveName()
}

// SetActiveSettings sets the active settings name.
func (m *Manager) SetActiveSettings(name string) error {
	return m.settings.SetActiveName(name)
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
	return m.validator.ValidateName(name)
}

func (m *Manager) normalizeSettingsName(name string) (string, error) {
	normalized, err := m.validator.NormalizeName(name)
	if err != nil {
		return "", fmt.Errorf("invalid settings name: %w", err)
	}
	return normalized, nil
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
	targetPath := m.paths.StoredSettingsPath(normalized)
	if exists, err := m.storage.Exists(targetPath); err != nil {
		return fmt.Errorf("failed to inspect target settings: %w", err)
	} else if !exists {
		return fmt.Errorf("settings '%s' not found", normalized)
	}
	if err := m.backup.BackupFile(m.paths.ActiveSettingsPath()); err != nil {
		return err
	}
	if err := m.storage.CopyFile(targetPath, m.paths.ActiveSettingsPath()); err != nil {
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
	activePath := m.paths.ActiveSettingsPath()
	if exists, err := m.storage.Exists(activePath); err != nil {
		return fmt.Errorf("failed to inspect settings.json: %w", err)
	} else if !exists {
		return errors.New("settings.json not found. Nothing to save.")
	}
	normalized, err := m.normalizeSettingsName(targetName)
	if err != nil {
		return err
	}
	targetPath := m.paths.StoredSettingsPath(normalized)
	if err := m.backup.BackupFile(targetPath); err != nil {
		return err
	}
	if err := m.storage.CopyFile(activePath, targetPath); err != nil {
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
	return m.settings.ListStored()
}

// ListEntry describes each available settings entry for list output.
type ListEntry = settings.ListEntry

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
	return m.settings.ListEntries(m.paths.ActiveSettingsPath(), m.CalculateHash)
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
	return m.backup.PruneBackups(olderThan)
}

// ActiveSettingsPath returns the path to settings.json for consumers like tests.
func (m *Manager) ActiveSettingsPath() string {
	return m.paths.ActiveSettingsPath()
}

// ActiveStatePath returns the path to settings.json.active for consumers like tests.
func (m *Manager) ActiveStatePath() string {
	return m.paths.ActiveStatePath()
}

// BackupDir returns the backup directory path.
func (m *Manager) BackupDir() string {
	return m.paths.BackupDir()
}

// SettingsStoreDir returns the store directory path.
func (m *Manager) SettingsStoreDir() string {
	return m.paths.SettingsStoreDir()
}

// FileSystem exposes the underlying filesystem.
func (m *Manager) FileSystem() afero.Fs {
	return m.storage.FileSystem()
}

// StoredSettingsPath returns the full path to a stored settings file.
func (m *Manager) StoredSettingsPath(name string) (string, error) {
	normalized, err := m.normalizeSettingsName(name)
	if err != nil {
		return "", err
	}
	return m.paths.StoredSettingsPath(normalized), nil
}

// SetNow overrides the clock used by the manager for testing.
func (m *Manager) SetNow(now func() time.Time) {
	m.backup.SetNow(now)
}
