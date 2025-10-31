package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/storage"
)

// Service handles settings persistence and retrieval operations.
type Service struct {
	storage       *storage.Storage
	settingsStore string
	activeState   string
}

// New creates a new settings Service.
func New(storage *storage.Storage, settingsStore, activeState string) *Service {
	return &Service{
		storage:       storage,
		settingsStore: settingsStore,
		activeState:   activeState,
	}
}

// GetActiveName returns the currently active settings name.
func (s *Service) GetActiveName() string {
	content, err := s.storage.ReadFile(s.activeState)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// SetActiveName sets the active settings name.
func (s *Service) SetActiveName(name string) error {
	return s.storage.WriteFile(s.activeState, []byte(name))
}

// ListStored returns the names of all stored settings profiles, sorted lexicographically.
//
// The function scans the settings store directory and returns
// only the base names (without .json extension) of regular files.
//
// Returns an error if the settings store directory cannot be read.
func (s *Service) ListStored() ([]string, error) {
	entries, err := s.storage.ReadDir(s.settingsStore)
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

// GetStoredPath returns the full path to a stored settings file.
func (s *Service) GetStoredPath(name string) string {
	return filepath.Join(s.settingsStore, name+".json")
}

// Exists checks if a settings profile exists.
func (s *Service) Exists(name string) (bool, error) {
	path := s.GetStoredPath(name)
	return s.storage.Exists(path)
}

// SettingsStoreDir returns the settings store directory path.
func (s *Service) SettingsStoreDir() string {
	return s.settingsStore
}

// ListEntry describes a settings entry for list output.
type ListEntry struct {
	Name       string
	Prefix     string
	Qualifiers []string
	Plain      bool
}

// ListEntries computes formatted entries for display in the list command.
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
func (s *Service) ListEntries(activePath string, calculateHash func(string) (string, error)) ([]ListEntry, error) {
	activeName := s.GetActiveName()
	currentHash, err := calculateHash(activePath)
	if err != nil {
		return nil, err
	}

	names, err := s.ListStored()
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
			storedHash, err := calculateHash(s.GetStoredPath(name))
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

// ReadFile reads a settings file.
func (s *Service) ReadFile(path string) ([]byte, error) {
	return s.storage.ReadFile(path)
}

// Stat returns file information.
func (s *Service) Stat(path string) (os.FileInfo, error) {
	return s.storage.Stat(path)
}
