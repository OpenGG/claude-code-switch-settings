# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Code Switcher (`ccs`) is a CLI utility that manages multiple `settings.json` profiles for Claude Code. See [README.md](README.md) for user documentation.

## Quick Commands

### Build & Test
```bash
go build -o ./bin/ccs ./cmd/ccs    # Build binary
go test ./...                       # Run all tests

# Check coverage (internal/ccs must be ≥80%)
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/... && \
  go tool cover -func=coverage.out | tail -1
```

### Code Quality (Required Before Commits)
```bash
gofmt -s -w .      # Format all Go files
go vet ./...       # Run static analysis (treat warnings as blockers)
```

## Architecture

The codebase follows a **clean layered architecture**. See [ARCHITECTURE.md](ARCHITECTURE.md) for comprehensive documentation.

### Directory Structure
```
internal/ccs/
├── domain/           # Core business errors
├── validator/        # Settings name validation
├── storage/          # Secure file operations (symlink protection, atomic writes)
├── backup/           # Content-addressed backups (SHA-256 deduplication)
├── settings/         # Settings persistence and retrieval
└── manager.go        # Thin orchestrator coordinating services
```

**Key Pattern:** Manager delegates to specialized services. All filesystem operations use `afero.Fs` for testability.

## Testing Conventions

**Philosophy:** Test quality > coverage numbers. See [TESTING.md](TESTING.md) for comprehensive guidelines.

### Quick Reference

**What to test:**
- ✅ Security (validator, symlink protection, permissions)
- ✅ Complex logic (backup deduplication, state machine, pruning)
- ✅ Integration (Use/Save/List workflows)

**What NOT to test:**
- ❌ Simple wrappers (ReadFile, WriteFile - tested via integration)
- ❌ Trivial getters/setters
- ❌ Third-party libraries

**Test naming:** `TestFunctionName_Scenario`
```go
func TestValidateName_NullBytes(t *testing.T) { ... }  // Attack prevention
func TestCopyFile_SecurePermissions(t *testing.T) { ... }  // Security requirement
```

**Helper pattern:**
```go
func newTestManager(t *testing.T) *Manager {
    t.Helper()
    fs := afero.NewMemMapFs()
    mgr := NewManager(fs, "/home/test", nil)  // nil logger = discard
    mgr.InitInfra()
    return mgr
}
```

**Coverage check:**
```bash
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/...
go tool cover -func=coverage.out | tail -1
```

**Target:** ~80% with meaningful tests (flexible guideline, not hard requirement)

See [TESTING.md](TESTING.md) for detailed testing philosophy, examples, and best practices.

## Release Process

Pushing a SemVer tag (e.g. `v1.0.0`) to `main` triggers `.github/workflows/release.yml`:
1. Runs `gofmt`, `go vet`, `go test`
2. Enforces 80% coverage threshold
3. Builds macOS binary
4. Publishes GitHub release with `ccs-macos-amd64.tar.gz`

## Key Implementation Details

- **Backup system**: SHA-256 content-addressed (see README.md "How Backups Work")
- **Security**: Owner-only permissions (0600/0700), symlink protection, atomic file operations
- **Validation**: Rejects path traversal, null bytes, invalid filesystem characters
- **CLI**: Uses Cobra for commands, promptui for interactive menus
