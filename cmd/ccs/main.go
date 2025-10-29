package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/afero"

	"github.com/example/claude-code-switch-settings/internal/ccs"
	"github.com/example/claude-code-switch-settings/internal/cli"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to determine home directory: %v", err)
	}

	if err := Run(afero.NewOsFs(), homeDir, cli.NewPromptUI(), os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func Run(fs afero.Fs, homeDir string, prompter cli.Prompter, stdout, stderr io.Writer, args []string) error {
	manager := ccs.NewManager(fs, homeDir)
	if err := manager.InitInfra(); err != nil {
		return fmt.Errorf("failed to initialize directories: %w", err)
	}

	root := cli.NewRootCommand(manager, prompter, stdout, stderr)
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs(args)

	return root.Execute()
}
