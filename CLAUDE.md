# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Code Switcher (`ccs`) is a CLI utility that manages multiple `settings.json` profiles for Claude Code. The tool maintains a repository of named settings in `~/.claude/switch-settings/`, tracks the active profile via `~/.claude/settings.json.active`, and creates content-addressed backups in `~/.claude/switch-settings-backup/` before any destructive operations.

## Commands

### Build & Run
- `go build -o ./bin/ccs ./cmd/ccs` - Build the binary
- `go run ./cmd/ccs --help` - Run directly without building
- `./bin/ccs <command>` - Execute built binary

### Code Quality
- `gofmt -l .` - List files needing formatting
- `gofmt -s -w .` - Format all Go files (required before commits)
- `go vet ./...` - Run static analysis (treat warnings as blockers)

### Testing
- `go test ./...` - Run all tests
- `go test ./internal/ccs` - Run tests for a specific package
- `go test -coverprofile=coverage.out ./...` - Generate coverage report
- `go tool cover -func=coverage.out` - View coverage by function (must be ≥80% total)
- `go test -v -run TestSpecificName` - Run a specific test

## Architecture

### Core Flow
The `main.go` entry point creates a `ccs.Manager` and wires it to Cobra commands in `internal/cli/command.go`. All filesystem operations flow through `afero.Fs` to enable testing with in-memory filesystems.

### Key Components

**Manager (internal/ccs/manager.go)**
- Central orchestrator for all profile operations
- Handles backup creation using MD5 content-addressing
- Validates profile names against POSIX and Windows filesystem rules
- All operations go through this single source of truth

**Paths (internal/ccs/paths.go)**
- Centralizes all path construction for the `~/.claude/` directory tree
- Three key locations:
  - `~/.claude/settings.json` - Active settings file
  - `~/.claude/switch-settings/` - Stored profiles
  - `~/.claude/switch-settings-backup/` - Content-addressed backups (MD5 hash filenames)

**CLI Commands (internal/cli/command.go)**
- Each command (list, use, save, prune-backups) is a separate Cobra command
- Interactive prompts use the `Prompter` interface for testability
- Commands construct user-facing output with qualifiers like `(active, modified)`

**Prompts (internal/cli/prompter.go, promptui_prompter.go)**
- Abstract interface for user interaction
- Production uses `promptui`, tests use fakes
- Supports Select (menus), Prompt (text input), and Confirm (yes/no)

### Backup System
Before any file overwrite, `Manager.backupFile()` computes an MD5 hash of the content. If a backup with that hash exists, only its modification time is refreshed. This prevents duplicate backups and allows `prune-backups` to use mtime for retention decisions.

### Testing Strategy
Tests sit beside production code (`*_test.go`) and use `afero.NewMemMapFs()` to avoid touching real directories. The `Manager.SetNow()` method allows tests to control time for prune operations. Table-driven tests cover profile name validation edge cases and command output variations.

## Release Process
Pushing a SemVer tag (e.g. `v1.0.0`) to `main` triggers `.github/workflows/release.yml`, which runs quality checks, enforces 80% coverage, builds a macOS binary, and publishes a GitHub release with `ccs-macos-amd64.tar.gz`.
