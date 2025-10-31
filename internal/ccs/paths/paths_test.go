package paths

import (
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	if pb == nil {
		t.Fatal("New() returned nil")
	}
	if pb.homeDir != homeDir {
		t.Errorf("New() homeDir = %q, want %q", pb.homeDir, homeDir)
	}
}

func TestClaudeDir(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	got := pb.ClaudeDir()
	want := filepath.Join(homeDir, ClaudeDirName)

	if got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
}

func TestActiveSettingsPath(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	got := pb.ActiveSettingsPath()
	want := filepath.Join(homeDir, ClaudeDirName, SettingsFileName)

	if got != want {
		t.Errorf("ActiveSettingsPath() = %q, want %q", got, want)
	}

	// Verify it builds upon ClaudeDir()
	expectedFromClaudeDir := filepath.Join(pb.ClaudeDir(), SettingsFileName)
	if got != expectedFromClaudeDir {
		t.Errorf("ActiveSettingsPath() doesn't build upon ClaudeDir()")
	}
}

func TestActiveStatePath(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	got := pb.ActiveStatePath()
	want := filepath.Join(homeDir, ClaudeDirName, ActiveFileName)

	if got != want {
		t.Errorf("ActiveStatePath() = %q, want %q", got, want)
	}

	// Verify it builds upon ClaudeDir()
	expectedFromClaudeDir := filepath.Join(pb.ClaudeDir(), ActiveFileName)
	if got != expectedFromClaudeDir {
		t.Errorf("ActiveStatePath() doesn't build upon ClaudeDir()")
	}
}

func TestSettingsStoreDir(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	got := pb.SettingsStoreDir()
	want := filepath.Join(homeDir, ClaudeDirName, StoreDirName)

	if got != want {
		t.Errorf("SettingsStoreDir() = %q, want %q", got, want)
	}

	// Verify it builds upon ClaudeDir()
	expectedFromClaudeDir := filepath.Join(pb.ClaudeDir(), StoreDirName)
	if got != expectedFromClaudeDir {
		t.Errorf("SettingsStoreDir() doesn't build upon ClaudeDir()")
	}
}

func TestBackupDir(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	got := pb.BackupDir()
	want := filepath.Join(homeDir, ClaudeDirName, BackupDirName)

	if got != want {
		t.Errorf("BackupDir() = %q, want %q", got, want)
	}

	// Verify it builds upon ClaudeDir()
	expectedFromClaudeDir := filepath.Join(pb.ClaudeDir(), BackupDirName)
	if got != expectedFromClaudeDir {
		t.Errorf("BackupDir() doesn't build upon ClaudeDir()")
	}
}

func TestStoredSettingsPath(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "work",
			expected: filepath.Join(homeDir, ClaudeDirName, StoreDirName, "work.json"),
		},
		{
			name:     "name with hyphen",
			input:    "work-mode",
			expected: filepath.Join(homeDir, ClaudeDirName, StoreDirName, "work-mode.json"),
		},
		{
			name:     "name with underscore",
			input:    "work_mode",
			expected: filepath.Join(homeDir, ClaudeDirName, StoreDirName, "work_mode.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pb.StoredSettingsPath(tt.input)
			if got != tt.expected {
				t.Errorf("StoredSettingsPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}

			// Verify it builds upon SettingsStoreDir()
			expectedFromStoreDir := filepath.Join(pb.SettingsStoreDir(), tt.input+".json")
			if got != expectedFromStoreDir {
				t.Errorf("StoredSettingsPath(%q) doesn't build upon SettingsStoreDir()", tt.input)
			}
		})
	}
}

// TestPathHierarchy verifies that paths build upon each other correctly
func TestPathHierarchy(t *testing.T) {
	homeDir := "/home/test"
	pb := New(homeDir)

	claudeDir := pb.ClaudeDir()

	// All paths should start with ClaudeDir
	paths := []struct {
		name string
		path string
	}{
		{"ActiveSettingsPath", pb.ActiveSettingsPath()},
		{"ActiveStatePath", pb.ActiveStatePath()},
		{"SettingsStoreDir", pb.SettingsStoreDir()},
		{"BackupDir", pb.BackupDir()},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.path) < len(claudeDir) || tt.path[:len(claudeDir)] != claudeDir {
				t.Errorf("%s = %q doesn't start with ClaudeDir = %q", tt.name, tt.path, claudeDir)
			}
		})
	}

	// StoredSettingsPath should start with SettingsStoreDir
	storeDir := pb.SettingsStoreDir()
	storedPath := pb.StoredSettingsPath("test")
	if len(storedPath) < len(storeDir) || storedPath[:len(storeDir)] != storeDir {
		t.Errorf("StoredSettingsPath = %q doesn't start with SettingsStoreDir = %q", storedPath, storeDir)
	}
}
