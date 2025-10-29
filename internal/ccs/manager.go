package ccs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/afero"
)

var (
	errNameEmpty        = errors.New("Name cannot be empty.")
	errNameDot          = errors.New("Name cannot be '.' or '..'.")
	errNameNonPrintable = errors.New("Name contains non-printable ASCII characters.")
	errNameInvalidChars = errors.New("Name contains invalid characters (<>:\"/|?*).")
	errNameReserved     = errors.New("Name is a reserved system filename.")
)

var reservedNamePattern = regexp.MustCompile(`^(?i)(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)
var invalidCharsPattern = regexp.MustCompile(`[<>:"/\\|?*]`)

// Manager coordinates settings operations using an injected filesystem and clock.
type Manager struct {
	fs      afero.Fs
	homeDir string
	now     func() time.Time
}

// NewManager constructs a Manager using the provided filesystem and home directory.
func NewManager(fs afero.Fs, homeDir string) *Manager {
	return &Manager{fs: fs, homeDir: homeDir, now: time.Now}
}

// InitInfra ensures that required directories exist.
func (m *Manager) InitInfra() error {
	paths := []string{m.claudeDir(), m.settingsStoreDir(), m.backupDir()}
	for _, p := range paths {
		if err := m.fs.MkdirAll(p, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", p, err)
		}
	}
	return nil
}

// CalculateMD5 returns the MD5 hash of the given file.
func (m *Manager) CalculateMD5(path string) (string, error) {
	info, err := m.fs.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("failed to stat file for hashing: %w", err)
	}
	if info.Size() == 0 {
		return "", nil
	}

	f, err := m.fs.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// backupFile copies the provided file into the backup directory.
func (m *Manager) backupFile(path string) (err error) {
	md5sum, err := m.CalculateMD5(path)
	if err != nil {
		return err
	}
	if md5sum == "" {
		return nil
	}

	source, err := m.fs.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to open file for backup: %w", err)
	}
	defer source.Close()

	backupPath := filepath.Join(m.backupDir(), md5sum+".json")
	now := m.now()
	if _, err := m.fs.Stat(backupPath); err == nil {
		if err := m.fs.Chtimes(backupPath, now, now); err != nil {
			return fmt.Errorf("failed to update backup timestamp: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat backup: %w", err)
	}

	dst, err := m.fs.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
	return afero.WriteFile(m.fs, m.activeStatePath(), []byte(name), 0o644)
}

// ValidateSettingsName validates the provided settings name.
func (m *Manager) ValidateSettingsName(name string) (bool, error) {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return false, errNameEmpty
	}
	if trimmed == "." || trimmed == ".." {
		return false, errNameDot
	}
	for _, r := range trimmed {
		if r < 0x20 || r > 0x7e {
			return false, errNameNonPrintable
		}
		if r == 0x7f {
			return false, errNameNonPrintable
		}
	}
	if invalidCharsPattern.MatchString(trimmed) {
		return false, errNameInvalidChars
	}
	if reservedNamePattern.MatchString(trimmed) {
		return false, errNameReserved
	}
	return true, nil
}

func (m *Manager) normalizeSettingsName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if ok, err := m.ValidateSettingsName(trimmed); !ok {
		if err != nil {
			return "", err
		}
		return "", errors.New("invalid settings name")
	}
	return trimmed, nil
}

// copyFile copies a file from src to dst, overwriting the destination.
func (m *Manager) copyFile(src, dst string) error {
	source, err := m.fs.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dir := filepath.Dir(dst)
	if err := m.fs.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	dest, err := m.fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()

	if copyErr != nil {
		m.fs.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		m.fs.Remove(tmp)
		return closeErr
	}

	if err := m.fs.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return m.fs.Rename(tmp, dst)
}

// Use activates the target settings by copying them into the active location.
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

// Save writes the current active settings to the specified target and activates it.
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

// StoredSettings returns the names of all stored settings sorted lexicographically.
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

// ListSettings computes the list entries for presentation.
func (m *Manager) ListSettings() ([]ListEntry, error) {
	if err := m.InitInfra(); err != nil {
		return nil, err
	}
	activeName := m.GetActiveSettingsName()
	currentMD5, err := m.CalculateMD5(m.activeSettingsPath())
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
			storedMD5, err := m.CalculateMD5(m.storedSettingsPath(name))
			if err != nil {
				return nil, err
			}
			entry.Qualifiers = append(entry.Qualifiers, "active")
			if currentMD5 != "" && storedMD5 != "" && currentMD5 != storedMD5 {
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
	} else if activeName == "" && currentMD5 != "" {
		entries = append(entries, ListEntry{
			Name:   "(Current settings.json is unsaved)",
			Prefix: "*",
			Plain:  true,
		})
	}

	return entries, nil
}

// PruneBackups removes backup files older than the specified cutoff.
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
