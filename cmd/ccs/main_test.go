package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/OpenGG/claude-code-switch-settings/internal/cli"
)

type noopPrompter struct{}

func (noopPrompter) Select(label string, items []string, defaultValue string) (int, string, error) {
	return 0, "", cliErr{}
}

func (noopPrompter) Prompt(label string) (string, error) {
	return "", cliErr{}
}

func (noopPrompter) Confirm(label string, defaultYes bool) (bool, error) {
	return false, cliErr{}
}

type cliErr struct{}

func (cliErr) Error() string { return "prompter should not be used" }

type cancelPrompter struct{}

func (cancelPrompter) Select(label string, items []string, defaultValue string) (int, string, error) {
	return 0, "", cli.ErrPromptCancelled
}

func (cancelPrompter) Prompt(label string) (string, error) {
	return "", cli.ErrPromptCancelled
}

func (cancelPrompter) Confirm(label string, defaultYes bool) (bool, error) {
	return false, cli.ErrPromptCancelled
}

func TestRunListCommand(t *testing.T) {
	fs := afero.NewMemMapFs()
	home := "/home/test"
	if err := fs.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{"list"}

	if err := Run(fs, home, noopPrompter{}, &stdout, &stderr, args); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if stdout.String() == "" {
		t.Fatalf("expected list output")
	}
}

func TestRunInitInfraError(t *testing.T) {
	roFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(roFs, "/home/ro", noopPrompter{}, &stdout, &stderr, nil); err == nil {
		t.Fatalf("expected init infra error")
	}
}

func TestRunPromptCancelled(t *testing.T) {
	fs := afero.NewMemMapFs()
	home := "/home/test"
	storeDir := filepath.Join(home, ".claude", "switch-settings")
	if err := fs.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	if err := afero.WriteFile(fs, filepath.Join(storeDir, "work.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stored settings: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Run(fs, home, cancelPrompter{}, &stdout, &stderr, []string{"use"})
	if !errors.Is(err, cli.ErrPromptCancelled) {
		t.Fatalf("expected prompt cancelled error, got %v", err)
	}
}
