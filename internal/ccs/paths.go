package ccs

import "path/filepath"

const (
	claudeDirName    = ".claude"
	settingsFileName = "settings.json"
	activeFileName   = "settings.json.active"
	storeDirName     = "switch-settings"
	backupDirName    = "switch-settings-backup"
)

func (m *Manager) claudeDir() string {
	return filepath.Join(m.homeDir, claudeDirName)
}

func (m *Manager) activeSettingsPath() string {
	return filepath.Join(m.claudeDir(), settingsFileName)
}

func (m *Manager) activeStatePath() string {
	return filepath.Join(m.claudeDir(), activeFileName)
}

func (m *Manager) settingsStoreDir() string {
	return filepath.Join(m.claudeDir(), storeDirName)
}

func (m *Manager) backupDir() string {
	return filepath.Join(m.claudeDir(), backupDirName)
}

func (m *Manager) storedSettingsPath(name string) string {
	return filepath.Join(m.settingsStoreDir(), name+".json")
}
