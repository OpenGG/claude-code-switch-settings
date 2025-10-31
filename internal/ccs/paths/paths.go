package paths

import "path/filepath"

// Directory and file name constants for Claude Code settings
const (
	ClaudeDirName    = ".claude"
	SettingsFileName = "settings.json"
	ActiveFileName   = "settings.json.active"
	StoreDirName     = "switch-settings"
	BackupDirName    = "switch-settings-backup"
)

// PathBuilder provides methods to construct Claude Code paths relative to a home directory.
type PathBuilder struct {
	homeDir string
}

// New creates a new PathBuilder for the given home directory.
func New(homeDir string) *PathBuilder {
	return &PathBuilder{homeDir: homeDir}
}

// ClaudeDir returns the .claude directory path.
func (p *PathBuilder) ClaudeDir() string {
	return filepath.Join(p.homeDir, ClaudeDirName)
}

// ActiveSettingsPath returns the path to the active settings.json file.
func (p *PathBuilder) ActiveSettingsPath() string {
	return filepath.Join(p.ClaudeDir(), SettingsFileName)
}

// ActiveStatePath returns the path to the active state file.
func (p *PathBuilder) ActiveStatePath() string {
	return filepath.Join(p.ClaudeDir(), ActiveFileName)
}

// SettingsStoreDir returns the directory where named settings profiles are stored.
func (p *PathBuilder) SettingsStoreDir() string {
	return filepath.Join(p.ClaudeDir(), StoreDirName)
}

// BackupDir returns the directory where backups are stored.
func (p *PathBuilder) BackupDir() string {
	return filepath.Join(p.ClaudeDir(), BackupDirName)
}

// StoredSettingsPath returns the path for a named settings profile.
func (p *PathBuilder) StoredSettingsPath(name string) string {
	return filepath.Join(p.SettingsStoreDir(), name+".json")
}
