package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/claude-code-switch-settings/internal/ccs"
)

func setupTestHome(t *testing.T) *ccs.Manager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCS_HOME", dir)
	mgr, err := ccs.NewManager()
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra error: %v", err)
	}
	return mgr
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	return data
}

func TestListCommand(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("content"))
	writeFile(t, mgr.ActiveSettingsPath(), []byte("content"))
	if err := mgr.SetActiveSettings("work"); err != nil {
		t.Fatalf("SetActiveSettings error: %v", err)
	}

	cmd := listCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("list command error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "* [work] (active)") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestListCommandEmpty(t *testing.T) {
	setupTestHome(t)
	cmd := listCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("list command error: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "No settings found. Use 'ccs save' to create one." {
		t.Fatalf("unexpected empty output: %s", buf.String())
	}
}

func TestUseCommandInteractive(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("content"))
	writeFile(t, mgr.ActiveSettingsPath(), []byte("old"))

	originalSelect := selectFunc
	defer func() { selectFunc = originalSelect }()
	selectFunc = func(label string, items []string) (int, string, error) {
		return 0, items[0], nil
	}

	cmd := useCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("use command error: %v", err)
	}

	if !strings.Contains(buf.String(), "Successfully switched") {
		t.Fatalf("expected success message, got %s", buf.String())
	}

	content := string(readFile(t, mgr.ActiveSettingsPath()))
	if content != "content" {
		t.Fatalf("expected active file to update, got %s", content)
	}
}

func TestUseCommandWithArgument(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("content"))
	cmd := useCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.RunE(cmd, []string{"work"}); err != nil {
		t.Fatalf("use command error: %v", err)
	}
	if !strings.Contains(buf.String(), "Successfully switched") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
	if string(readFile(t, mgr.ActiveSettingsPath())) != "content" {
		t.Fatalf("expected settings.json to match store")
	}
}

func TestUseCommandNoStoredSettings(t *testing.T) {
	setupTestHome(t)
	cmd := useCommand()
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatalf("expected error when no stored settings")
	}
}

func TestSaveCommandNewAndOverwrite(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, mgr.ActiveSettingsPath(), []byte("current"))
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "personal.json"), []byte("old"))

	seq := []string{"[New Settings]", "personal"}
	selectIndex := 0
	originalSelect := selectFunc
	defer func() { selectFunc = originalSelect }()
	selectFunc = func(label string, items []string) (int, string, error) {
		choice := seq[selectIndex]
		selectIndex++
		return 0, choice, nil
	}

	responses := []struct {
		text      string
		isConfirm bool
	}{
		{"bad/name", false},
		{"dev", false},
		{"y", true},
	}
	respIndex := 0
	originalPrompt := promptFunc
	defer func() { promptFunc = originalPrompt }()
	promptFunc = func(label string, isConfirm bool) (string, error) {
		resp := responses[respIndex]
		respIndex++
		if resp.isConfirm != isConfirm {
			t.Fatalf("unexpected prompt type for %s", label)
		}
		return resp.text, nil
	}

	cmd := saveCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("save command error: %v", err)
	}

	if string(readFile(t, filepath.Join(mgr.SettingsStoreDir(), "dev.json"))) != "current" {
		t.Fatalf("expected new settings file to be created")
	}

	// run overwrite branch
	time.Sleep(10 * time.Millisecond)
	writeFile(t, mgr.ActiveSettingsPath(), []byte("next"))
	selectIndex = 1
	respIndex = 2
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("save overwrite error: %v", err)
	}

	if string(readFile(t, filepath.Join(mgr.SettingsStoreDir(), "personal.json"))) != "next" {
		t.Fatalf("expected personal.json to update")
	}
}

func TestSaveCommandRequiresActiveFile(t *testing.T) {
	setupTestHome(t)
	cmd := saveCommand()
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatalf("expected error when settings.json is missing")
	}
}

func TestSaveCommandOverwriteCancelled(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, mgr.ActiveSettingsPath(), []byte("current"))
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "personal.json"), []byte("old"))

	originalSelect := selectFunc
	defer func() { selectFunc = originalSelect }()
	selectFunc = func(label string, items []string) (int, string, error) {
		return 0, "personal", nil
	}

	originalPrompt := promptFunc
	defer func() { promptFunc = originalPrompt }()
	promptFunc = func(label string, isConfirm bool) (string, error) {
		if !isConfirm {
			return "", nil
		}
		return "n", nil
	}

	cmd := saveCommand()
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestPruneBackupsCommand(t *testing.T) {
	mgr := setupTestHome(t)
	oldFile := filepath.Join(mgr.BackupDir(), "old.json")
	writeFile(t, oldFile, []byte("old"))
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	originalPrompt := promptFunc
	defer func() { promptFunc = originalPrompt }()
	promptFunc = func(label string, isConfirm bool) (string, error) {
		return "y", nil
	}

	cmd := pruneBackupsCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.Flags().Set("older-than", "24h")
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("prune command error: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected backup to be removed")
	}
}

func TestPruneBackupsCommandForce(t *testing.T) {
	mgr := setupTestHome(t)
	oldFile := filepath.Join(mgr.BackupDir(), "old.json")
	writeFile(t, oldFile, []byte("old"))
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	cmd := pruneBackupsCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.Flags().Set("older-than", "24h")
	cmd.Flags().Set("force", "true")
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("prune command error: %v", err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected forced prune to remove file")
	}
}

func TestPruneBackupsCommandCancelled(t *testing.T) {
	setupTestHome(t)
	originalPrompt := promptFunc
	defer func() { promptFunc = originalPrompt }()
	promptFunc = func(label string, isConfirm bool) (string, error) {
		return "n", nil
	}

	cmd := pruneBackupsCommand()
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestMainExecutesWithoutExit(t *testing.T) {
	mgr := setupTestHome(t)
	writeFile(t, filepath.Join(mgr.SettingsStoreDir(), "work.json"), []byte("content"))
	writeFile(t, mgr.ActiveSettingsPath(), []byte("content"))
	if err := mgr.SetActiveSettings("work"); err != nil {
		t.Fatalf("SetActiveSettings error: %v", err)
	}

	rootCmd.SetArgs([]string{"list"})
	defer rootCmd.SetArgs(nil)
	oldOut := rootCmd.OutOrStdout()
	oldErr := rootCmd.ErrOrStderr()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	defer rootCmd.SetOut(oldOut)
	defer rootCmd.SetErr(oldErr)

	called := false
	oldExit := exitFunc
	exitFunc = func(code int) { called = true }
	defer func() { exitFunc = oldExit }()

	main()

	if called {
		t.Fatalf("exit should not be invoked on successful execution")
	}
}
