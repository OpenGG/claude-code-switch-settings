package ccs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func setupCLIEnv(t *testing.T) (*Service, string) {
	t.Helper()
	tempDir := t.TempDir()
	base := filepath.Join(tempDir, ".claude")
	oldFS := fsProvider
	fsProvider = afero.NewOsFs()
	t.Cleanup(func() { fsProvider = oldFS })
	oldFactory := rootFactory
	rootFactory = func() *cobra.Command { return newRootCmd() }
	t.Cleanup(func() { rootFactory = oldFactory })
	oldSelect := selectRunner
	oldPrompt := promptRunner
	selectRunner = func(sel *promptui.Select) (int, string, error) { return sel.Run() }
	promptRunner = func(pr *promptui.Prompt) (string, error) { return pr.Run() }
	t.Cleanup(func() {
		selectRunner = oldSelect
		promptRunner = oldPrompt
	})

	if err := fsProvider.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}

	service, err := NewService(fsProvider, base)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Setenv("CCS_BASE_DIR", base)
	t.Setenv("CCS_NON_INTERACTIVE", "1")
	return service, base
}

func TestResolveOlderThan(t *testing.T) {
	duration, err := resolveOlderThan("30d")
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if duration != 30*24*time.Hour {
		t.Fatalf("unexpected duration: %v", duration)
	}

	if _, err := resolveOlderThan("5x"); err == nil {
		t.Fatalf("expected error for invalid suffix")
	}
}

func TestHumanizeDuration(t *testing.T) {
	if humanizeDuration(48*time.Hour) != "2d" {
		t.Fatalf("unexpected humanized duration")
	}
	if !strings.Contains(humanizeDuration(3*time.Hour), "3h") {
		t.Fatalf("expected hour representation")
	}
}

func TestResolveBaseDirOverride(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".custom-claude")
	os.Setenv("CCS_BASE_DIR", base)
	defer os.Unsetenv("CCS_BASE_DIR")

	resolved, err := resolveBaseDir()
	if err != nil {
		t.Fatalf("resolveBaseDir failed: %v", err)
	}
	if resolved != base {
		t.Fatalf("expected override base dir")
	}
}

func TestListCommand(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	if err := afero.WriteFile(fsProvider, filepath.Join(base, "settings.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}
	if err := service.SaveSettings("work"); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "* [work] (active)") {
		t.Fatalf("expected active work entry: %s", output)
	}
}

func TestUseCommand(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to write stored settings: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "settings.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write active settings: %v", err)
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"use", "work"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	content, err := afero.ReadFile(fsProvider, filepath.Join(base, "settings.json"))
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	if string(content) != "work" {
		t.Fatalf("expected settings switched to work")
	}
}

func TestSaveCommandNewSetting(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	if err := afero.WriteFile(fsProvider, filepath.Join(base, "settings.json"), []byte("dev"), 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	t.Setenv("CCS_NEW_SETTINGS_NAME", "dev-profile")
	t.Setenv("CCS_SELECT_SETTING", "[New Settings]")

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"save"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	content, err := afero.ReadFile(fsProvider, filepath.Join(base, "switch-settings", "dev-profile.json"))
	if err != nil {
		t.Fatalf("failed to read saved settings: %v", err)
	}
	if string(content) != "dev" {
		t.Fatalf("expected saved content")
	}
}

func TestPruneBackupsCommand(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}

	oldPath := filepath.Join(base, "switch-settings-backup", "old.json")
	recentPath := filepath.Join(base, "switch-settings-backup", "recent.json")

	if err := afero.WriteFile(fsProvider, oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write old backup: %v", err)
	}
	if err := afero.WriteFile(fsProvider, recentPath, []byte("recent"), 0o644); err != nil {
		t.Fatalf("failed to write recent backup: %v", err)
	}

	oldTime := time.Now().Add(-90 * 24 * time.Hour)
	recentTime := time.Now().Add(-5 * 24 * time.Hour)
	if err := fsProvider.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set old backup time: %v", err)
	}
	if err := fsProvider.Chtimes(recentPath, recentTime, recentTime); err != nil {
		t.Fatalf("failed to set recent backup time: %v", err)
	}

	t.Setenv("CCS_PRUNE_DURATION", "30d")

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"prune-backups"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if _, err := fsProvider.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old backup removed")
	}
	if _, err := fsProvider.Stat(recentPath); err != nil {
		t.Fatalf("expected recent backup to remain: %v", err)
	}
}

func TestExecuteHandlesError(t *testing.T) {
	oldExit := exitFunc
	exitCode := 0
	exitFunc = func(code int) {
		exitCode = code
	}
	defer func() { exitFunc = oldExit }()

	rootFactory = func() *cobra.Command {
		return &cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("boom")
			},
		}
	}
	defer func() { rootFactory = func() *cobra.Command { return newRootCmd() } }()

	Execute()

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestPromptForSettingsNameAutomation(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}

	name, err := promptForSettingsName(service, "Select")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if name != "work" {
		t.Fatalf("expected automation to pick first entry")
	}
}

func TestPromptForSettingsNameInteractive(t *testing.T) {
	service, base := setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "dev.json"), []byte("dev"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}

	selectRunner = func(sel *promptui.Select) (int, string, error) {
		return 0, "dev", nil
	}

	name, err := promptForSettingsName(service, "Select")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if name != "dev" {
		t.Fatalf("expected interactive selection")
	}
}

func TestPromptForNewSettingsNameAutomation(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "dev.json"), []byte("dev"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}
	t.Setenv("CCS_NEW_SETTINGS_NAME", "qa")

	name, err := promptForNewSettingsName(service)
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if name != "qa" {
		t.Fatalf("expected automation to return provided name")
	}
}

func TestPromptForNewSettingsNameInteractive(t *testing.T) {
	service, base := setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "existing.json"), []byte("existing"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}

	responses := []string{"existing", "new-profile"}
	index := 0
	promptRunner = func(pr *promptui.Prompt) (string, error) {
		value := responses[index]
		index++
		return value, nil
	}

	name, err := promptForNewSettingsName(service)
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if name != "new-profile" {
		t.Fatalf("expected second response to be accepted")
	}
}

func TestPromptForPruneDurationInteractive(t *testing.T) {
	setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	selectRunner = func(sel *promptui.Select) (int, string, error) {
		return 1, "60 days", nil
	}

	duration, err := promptForPruneDuration()
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if duration != 60*24*time.Hour {
		t.Fatalf("expected duration to match selection")
	}
}

func TestUseCommandInteractiveSelection(t *testing.T) {
	service, base := setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "settings.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write active settings: %v", err)
	}

	selectRunner = func(sel *promptui.Select) (int, string, error) {
		return 0, "work", nil
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"use"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func TestSaveCommandOverwriteExisting(t *testing.T) {
	service, base := setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "prod.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write stored settings: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "settings.json"), []byte("new"), 0o644); err != nil {
		t.Fatalf("failed to write active settings: %v", err)
	}

	selectRunner = func(sel *promptui.Select) (int, string, error) {
		return 0, "prod", nil
	}
	promptRunner = func(pr *promptui.Prompt) (string, error) {
		return "y", nil
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"save"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func TestListCommandEmpty(t *testing.T) {
	_, _ = setupCLIEnv(t)
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(buf.String(), "No settings stored yet.") {
		t.Fatalf("expected empty message: %s", buf.String())
	}
}

func TestPruneBackupsCommandInteractive(t *testing.T) {
	service, base := setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	oldPath := filepath.Join(base, "switch-settings-backup", "old.json")
	if err := afero.WriteFile(fsProvider, oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write old backup: %v", err)
	}
	past := time.Now().Add(-90 * 24 * time.Hour)
	if err := fsProvider.Chtimes(oldPath, past, past); err != nil {
		t.Fatalf("failed to set modtime: %v", err)
	}

	promptRunner = func(pr *promptui.Prompt) (string, error) {
		return "y", nil
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"prune-backups", "--older-than", "30d"})

	if err := root.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func TestResolveBaseDirDefault(t *testing.T) {
	os.Unsetenv("CCS_BASE_DIR")
	dir, err := resolveBaseDir()
	if err != nil {
		t.Fatalf("resolveBaseDir failed: %v", err)
	}
	if !strings.HasSuffix(dir, ".claude") {
		t.Fatalf("expected directory to end with .claude: %s", dir)
	}
}

func TestPromptForSettingsNameAutomationMissingSelection(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "work.json"), []byte("work"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}
	t.Setenv("CCS_SELECT_SETTING", "missing")

	if _, err := promptForSettingsName(service, "Select"); err == nil {
		t.Fatalf("expected error when automation selection missing")
	}
}

func TestPromptForNewSettingsNameAutomationDuplicate(t *testing.T) {
	service, base := setupCLIEnv(t)
	if err := service.InitInfra(); err != nil {
		t.Fatalf("InitInfra failed: %v", err)
	}
	if err := afero.WriteFile(fsProvider, filepath.Join(base, "switch-settings", "dev.json"), []byte("dev"), 0o644); err != nil {
		t.Fatalf("failed to create stored settings: %v", err)
	}
	t.Setenv("CCS_NEW_SETTINGS_NAME", "dev")

	if _, err := promptForNewSettingsName(service); err == nil {
		t.Fatalf("expected error when automation duplicates name")
	}
}

func TestUseCommandNoStoredSettings(t *testing.T) {
	setupCLIEnv(t)
	t.Setenv("CCS_NON_INTERACTIVE", "")
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"use"})

	if err := root.Execute(); err == nil {
		t.Fatalf("expected error when no stored settings")
	}
}

func TestSaveCommandMissingSettingsFile(t *testing.T) {
	setupCLIEnv(t)
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"save"})

	if err := root.Execute(); err == nil {
		t.Fatalf("expected error when settings.json missing")
	}
}

func TestPruneBackupsInvalidDuration(t *testing.T) {
	setupCLIEnv(t)
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"prune-backups", "--older-than", "oops"})

	if err := root.Execute(); err == nil {
		t.Fatalf("expected error for invalid duration")
	}
}
