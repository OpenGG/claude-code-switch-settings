package ccs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	claudeDirName        = ".claude"
	settingsFileName     = "settings.json"
	activeStateFileName  = "settings.json.active"
	settingsStoreDirName = "switch-settings"
	backupDirName        = "switch-settings-backup"
)

// Service contains the core filesystem logic for managing Claude Code settings.
type Service struct {
	fs      afero.Fs
	nowFunc func() time.Time
	baseDir string
}

// Paths encapsulates important filesystem paths used by the service.
type Paths struct {
	BaseDir            string
	ActiveSettingsPath string
	ActiveStatePath    string
	SettingsStoreDir   string
	BackupDir          string
}

// NewService creates a new Service using the provided filesystem and base directory.
func NewService(fs afero.Fs, baseDir string) (*Service, error) {
	if fs == nil {
		return nil, errors.New("filesystem cannot be nil")
	}
	if baseDir == "" {
		return nil, errors.New("baseDir cannot be empty")
	}
	return &Service{fs: fs, nowFunc: time.Now, baseDir: baseDir}, nil
}

// Paths returns the computed paths for the service.
func (s *Service) Paths() Paths {
	return Paths{
		BaseDir:            s.baseDir,
		ActiveSettingsPath: filepath.Join(s.baseDir, settingsFileName),
		ActiveStatePath:    filepath.Join(s.baseDir, activeStateFileName),
		SettingsStoreDir:   filepath.Join(s.baseDir, settingsStoreDirName),
		BackupDir:          filepath.Join(s.baseDir, backupDirName),
	}
}

// InitInfra ensures that the storage and backup directories exist.
func (s *Service) InitInfra() error {
	paths := s.Paths()
	if err := s.fs.MkdirAll(paths.SettingsStoreDir, 0o755); err != nil {
		return fmt.Errorf("failed to create settings store directory: %w", err)
	}
	if err := s.fs.MkdirAll(paths.BackupDir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	return nil
}

// CalculateMD5 computes the MD5 checksum of the specified file.
func (s *Service) CalculateMD5(path string) (string, error) {
	file, err := s.fs.Open(path)
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// BackupFile saves a copy of the provided file to the backup directory.
func (s *Service) BackupFile(path string) error {
	md5sum, err := s.CalculateMD5(path)
	if err != nil {
		return fmt.Errorf("failed to calculate md5: %w", err)
	}
	if md5sum == "" {
		return nil
	}

	paths := s.Paths()
	backupPath := filepath.Join(paths.BackupDir, md5sum+".json")

	if _, err := s.fs.Stat(backupPath); err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			if err := s.copyFile(path, backupPath); err != nil {
				return fmt.Errorf("failed to create backup: %w", err)
			}
		} else {
			return fmt.Errorf("failed to stat backup: %w", err)
		}
	}

	now := s.nowFunc()
	return s.fs.Chtimes(backupPath, now, now)
}

func (s *Service) copyFile(src, dst string) error {
	data, err := afero.ReadFile(s.fs, src)
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return nil
		}
		return err
	}

	if err := s.fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tempPath := filepath.Join(filepath.Dir(dst), fmt.Sprintf("ccs-tmp-%d", s.nowFunc().UnixNano()))
	if err := afero.WriteFile(s.fs, tempPath, data, 0o644); err != nil {
		return err
	}

	if err := s.fs.Rename(tempPath, dst); err != nil {
		s.fs.Remove(tempPath)
		return err
	}

	return nil
}

// GetActiveSettingsName returns the active settings name from the state file.
func (s *Service) GetActiveSettingsName() (string, error) {
	paths := s.Paths()
	data, err := afero.ReadFile(s.fs, paths.ActiveStatePath)
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SetActiveSettings writes the active settings name to the state file.
func (s *Service) SetActiveSettings(name string) error {
	paths := s.Paths()
	if err := afero.WriteFile(s.fs, paths.ActiveStatePath, []byte(name), 0o644); err != nil {
		return fmt.Errorf("failed to write active settings file: %w", err)
	}
	return nil
}

// ValidateSettingsName ensures the provided settings name is safe for cross-platform use.
func ValidateSettingsName(name string) (bool, error) {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return false, errors.New("Name cannot be empty.")
	}
	if trimmed == "." || trimmed == ".." {
		return false, errors.New("Name cannot be '.' or '..'.")
	}
	for _, r := range trimmed {
		if r < 0x20 || r > 0x7e || r == 0x7f {
			return false, errors.New("Name contains non-printable ASCII characters.")
		}
	}
	if strings.ContainsAny(trimmed, "<>:\"/\\|?*") {
		return false, errors.New("Name contains invalid characters (<>:\"/|?*).")
	}
	upper := strings.ToUpper(trimmed)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return false, errors.New("Name is a reserved system filename.")
	}
	if strings.HasPrefix(upper, "COM") && len(upper) == 4 {
		suffix := upper[3]
		if suffix >= '1' && suffix <= '9' {
			return false, errors.New("Name is a reserved system filename.")
		}
	}
	if strings.HasPrefix(upper, "LPT") && len(upper) == 4 {
		suffix := upper[3]
		if suffix >= '1' && suffix <= '9' {
			return false, errors.New("Name is a reserved system filename.")
		}
	}
	return true, nil
}

// SettingsEntry represents a saved settings file.
type SettingsEntry struct {
	Name     string
	IsActive bool
	Modified bool
}

// ListSettings returns the available settings entries along with additional state lines.
func (s *Service) ListSettings() ([]string, error) {
	if err := s.InitInfra(); err != nil {
		return nil, err
	}
	paths := s.Paths()

	activeName, err := s.GetActiveSettingsName()
	if err != nil {
		return nil, err
	}

	_, err = s.fs.Stat(paths.ActiveSettingsPath)
	currentExists := err == nil
	if err != nil && !errors.Is(err, afero.ErrFileNotFound) {
		return nil, err
	}

	currentMD5 := ""
	if currentExists {
		currentMD5, err = s.CalculateMD5(paths.ActiveSettingsPath)
		if err != nil {
			return nil, err
		}
	}

	entries := []string{}

	files, err := afero.ReadDir(s.fs, paths.SettingsStoreDir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(file.Name(), ".json")
		names = append(names, name)
	}

	sort.Strings(names)

	handledActive := false
	for _, name := range names {
		line := fmt.Sprintf("  [%s]", name)
		if name == activeName {
			handledActive = true
			storedMD5, err := s.CalculateMD5(filepath.Join(paths.SettingsStoreDir, name+".json"))
			if err != nil {
				return nil, err
			}
			if storedMD5 == currentMD5 {
				line = fmt.Sprintf("* [%s] (active)", name)
			} else {
				line = fmt.Sprintf("* [%s] (active, modified)", name)
			}
		}
		entries = append(entries, line)
	}

	if activeName != "" && !handledActive {
		entries = append(entries, fmt.Sprintf("! [%s] (active, missing!)", activeName))
	}

	if activeName == "" && currentExists {
		entries = append(entries, "* (Current settings.json is unsaved)")
	}

	return entries, nil
}

// UseSettings loads the named settings file and activates it.
func (s *Service) UseSettings(name string) error {
	if err := s.InitInfra(); err != nil {
		return err
	}
	paths := s.Paths()
	targetPath := filepath.Join(paths.SettingsStoreDir, name+".json")
	if _, err := s.fs.Stat(targetPath); err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return fmt.Errorf("Settings '%s' not found", name)
		}
		return err
	}

	if err := s.BackupFile(paths.ActiveSettingsPath); err != nil {
		return err
	}

	if err := s.copyFile(targetPath, paths.ActiveSettingsPath); err != nil {
		return fmt.Errorf("failed to apply settings: %w", err)
	}

	if err := s.SetActiveSettings(name); err != nil {
		return err
	}

	return nil
}

// SaveSettings saves the active settings.json to the store under the provided name.
func (s *Service) SaveSettings(name string) error {
	if err := s.InitInfra(); err != nil {
		return err
	}
	paths := s.Paths()

	if _, err := s.fs.Stat(paths.ActiveSettingsPath); err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return errors.New("settings.json not found. Nothing to save")
		}
		return err
	}

	targetPath := filepath.Join(paths.SettingsStoreDir, name+".json")
	if err := s.BackupFile(targetPath); err != nil {
		return err
	}

	if err := s.copyFile(paths.ActiveSettingsPath, targetPath); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	if err := s.SetActiveSettings(name); err != nil {
		return err
	}

	return nil
}

// PruneBackups removes backup files that are older than the provided cutoff time.
func (s *Service) PruneBackups(olderThan time.Duration) (int, error) {
	if err := s.InitInfra(); err != nil {
		return 0, err
	}
	paths := s.Paths()

	files, err := afero.ReadDir(s.fs, paths.BackupDir)
	if err != nil {
		return 0, err
	}

	now := s.nowFunc()
	cutoff := now.Add(-olderThan)
	removed := 0

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := s.fs.Stat(filepath.Join(paths.BackupDir, file.Name()))
		if err != nil {
			return removed, err
		}
		if info.ModTime().Before(cutoff) {
			if err := s.fs.Remove(filepath.Join(paths.BackupDir, file.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}

	return removed, nil
}

// ListStoredNames returns the names of all stored settings.
func (s *Service) ListStoredNames() ([]string, error) {
	if err := s.InitInfra(); err != nil {
		return nil, err
	}
	paths := s.Paths()
	files, err := afero.ReadDir(s.fs, paths.SettingsStoreDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		names = append(names, strings.TrimSuffix(file.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}
