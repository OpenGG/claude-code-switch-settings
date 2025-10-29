package main

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"
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
