package ccs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager coordinates file operations for Claude Code settings switching.
type Manager struct {
	baseDir string
}

// NewManager constructs a Manager rooted at the Claude configuration directory.
func NewManager() (*Manager, error) {
	base, err := resolveClaudeDir()
	if err != nil {
		return nil, err
	}
	return &Manager{baseDir: base}, nil
}

// BaseDir returns the Claude configuration directory path.
func (m *Manager) BaseDir() string {
	return m.baseDir
}

// ActiveSettingsPath returns the path to the active settings.json file.
func (m *Manager) ActiveSettingsPath() string {
	return filepath.Join(m.baseDir, "settings.json")
}

// ActiveStatePath returns the path to the state file storing the active settings name.
func (m *Manager) ActiveStatePath() string {
	return filepath.Join(m.baseDir, "settings.json.active")
}

// SettingsStoreDir returns the directory that stores named settings.
func (m *Manager) SettingsStoreDir() string {
	return filepath.Join(m.baseDir, "switch-settings")
}

// BackupDir returns the directory where backups are stored.
func (m *Manager) BackupDir() string {
	return filepath.Join(m.baseDir, "switch-settings-backup")
}

// InitInfra ensures that required directories exist.
func (m *Manager) InitInfra() error {
	storeDir := m.SettingsStoreDir()
	backupDir := m.BackupDir()
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("failed to create settings store directory: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	return nil
}

// CalculateMD5 returns the MD5 hash of a file. When the file does not exist, an empty string is returned without an error.
func (m *Manager) CalculateMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", nil
	}

	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BackupFile copies the specified file into the backup directory using its MD5 as the filename.
func (m *Manager) BackupFile(path string) error {
	md5sum, err := m.CalculateMD5(path)
	if err != nil {
		return err
	}
	if md5sum == "" {
		return nil
	}

	source, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer source.Close()

	backupPath := filepath.Join(m.BackupDir(), md5sum+".json")
	now := time.Now()

	if _, err := os.Stat(backupPath); err == nil {
		return os.Chtimes(backupPath, now, now)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	destination, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	return os.Chtimes(backupPath, now, now)
}

// GetActiveSettingsName reads the active settings name from the state file.
func (m *Manager) GetActiveSettingsName() (string, error) {
	bytes, err := os.ReadFile(m.ActiveStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}

// SetActiveSettings writes the provided name into the active state file.
func (m *Manager) SetActiveSettings(name string) error {
	return os.WriteFile(m.ActiveStatePath(), []byte(name), 0o644)
}

// ValidateSettingsName ensures the provided name is portable across filesystems.
func ValidateSettingsName(name string) (bool, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false, fmt.Errorf("name cannot be empty")
	}
	if trimmed == "." || trimmed == ".." {
		return false, fmt.Errorf("name cannot be '.' or '..'")
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f || r > 0x7e {
			return false, fmt.Errorf("name contains non-printable ASCII characters")
		}
	}
	if strings.ContainsAny(trimmed, "<>:\"/\\|?*") {
		return false, fmt.Errorf("name contains invalid characters (<>:\"/\\|?*)")
	}
	upper := strings.ToUpper(trimmed)
	reserved := map[string]struct{}{
		"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
		"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
		"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
	}
	if _, exists := reserved[upper]; exists {
		return false, fmt.Errorf("name is a reserved system filename")
	}
	return true, nil
}

// ListEntry represents a single settings entry and its display state.
type ListEntry struct {
	Display string
}

// ListSettings produces the formatted list strings for the list command.
func (m *Manager) ListSettings() ([]ListEntry, error) {
	if err := m.InitInfra(); err != nil {
		return nil, err
	}
	activeName, err := m.GetActiveSettingsName()
	if err != nil {
		return nil, err
	}

	activeSettingsPath := m.ActiveSettingsPath()
	currentMD5, err := m.CalculateMD5(activeSettingsPath)
	if err != nil {
		return nil, err
	}
	activeExists := false
	if _, err := os.Stat(activeSettingsPath); err == nil {
		activeExists = true
	}

	entries, err := os.ReadDir(m.SettingsStoreDir())
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(names)

	var result []ListEntry
	isActiveStateHandled := false

	for _, name := range names {
		storedPath := filepath.Join(m.SettingsStoreDir(), name+".json")
		if name == activeName {
			isActiveStateHandled = true
			storedMD5, err := m.CalculateMD5(storedPath)
			if err != nil {
				return nil, err
			}
			if storedMD5 != "" && storedMD5 == currentMD5 {
				result = append(result, ListEntry{Display: fmt.Sprintf("* [%s] (active)", name)})
			} else {
				result = append(result, ListEntry{Display: fmt.Sprintf("* [%s] (active, modified)", name)})
			}
			continue
		}
		result = append(result, ListEntry{Display: fmt.Sprintf("  [%s]", name)})
	}

	if activeName != "" && !isActiveStateHandled {
		result = append(result, ListEntry{Display: fmt.Sprintf("! [%s] (active, missing!)", activeName)})
	} else if activeName == "" && activeExists && currentMD5 != "" {
		result = append(result, ListEntry{Display: "* (Current settings.json is unsaved)"})
	}

	return result, nil
}

// UseSettings activates the specified named settings.
func (m *Manager) UseSettings(name string) error {
	if err := m.InitInfra(); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("settings name is required")
	}

	targetPath := filepath.Join(m.SettingsStoreDir(), trimmed+".json")
	if _, err := os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("settings '%s' not found", trimmed)
		}
		return err
	}

	if err := m.BackupFile(m.ActiveSettingsPath()); err != nil {
		return err
	}

	if err := copyFile(targetPath, m.ActiveSettingsPath()); err != nil {
		return err
	}

	return m.SetActiveSettings(trimmed)
}

// SaveSettings copies the current active settings into the named archive entry and marks it active.
func (m *Manager) SaveSettings(targetName string) error {
	if err := m.InitInfra(); err != nil {
		return err
	}
	sourcePath := m.ActiveSettingsPath()
	if _, err := os.Stat(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("settings.json not found. Nothing to save")
		}
		return err
	}

	trimmed := strings.TrimSpace(targetName)
	if trimmed == "" {
		return fmt.Errorf("target settings name is required")
	}
	targetPath := filepath.Join(m.SettingsStoreDir(), trimmed+".json")

	if err := m.BackupFile(targetPath); err != nil {
		return err
	}

	if err := copyFile(sourcePath, targetPath); err != nil {
		return err
	}

	return m.SetActiveSettings(trimmed)
}

// PruneBackups removes backup files older than the provided duration and returns the number removed.
func (m *Manager) PruneBackups(olderThan time.Duration) (int, error) {
	if err := m.InitInfra(); err != nil {
		return 0, err
	}

	entries, err := os.ReadDir(m.BackupDir())
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-olderThan)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(m.BackupDir(), entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			return removed, err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	now := time.Now()
	return os.Chtimes(destination, now, now)
}

func resolveClaudeDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CCS_HOME")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}
