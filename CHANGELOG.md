# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Architecture

#### Clean Architecture Refactoring
The codebase has been refactored from a monolithic Manager (~400 lines) into a clean layered architecture with focused services (~500 lines total, better organized):

**New Structure**:
- **Domain Layer** (`internal/ccs/domain/`) - Core business errors
- **Validator Service** (`internal/ccs/validator/`) - Name validation logic (60 lines)
- **Storage Service** (`internal/ccs/storage/`) - Secure file operations (140 lines)
- **Backup Service** (`internal/ccs/backup/`) - Content-addressed backups (200 lines)
- **Settings Service** (`internal/ccs/settings/`) - Settings persistence (120 lines)
- **Manager** - Thin orchestrator coordinating services (120 lines)

**Benefits**:
- ✅ **Single Responsibility**: Each service has one clear purpose
- ✅ **Testability**: Services can be tested in isolation
- ✅ **Maintainability**: Clear boundaries between concerns
- ✅ **Security**: Defense-in-depth with multiple validation layers
- ✅ **Backward Compatible**: Public API unchanged

**Test Coverage**: Improved to 81.0% for core logic (from 78.8%)

See [ARCHITECTURE.md](ARCHITECTURE.md) for complete documentation.

### Added
- **Structured logging** with Go's `log/slog` for better observability and debugging
- **Empty file backup handling** - Empty settings files are now backed up with a warning logged instead of being silently skipped
- **Exported error variables** (`ErrSettingsNameEmpty`, `ErrSettingsNameNullByte`, etc.) allowing callers to use `errors.Is()` for better error handling
- **Null byte validation** - Settings names containing null bytes are now explicitly rejected to prevent path traversal attacks
- **Comprehensive godoc comments** on all public APIs with usage examples
- **Early input validation** in CLI commands - validation now happens before passing to Manager for better error messages
- **Magic number extraction** - Hardcoded values like menu size are now named constants
- **Simplified slice operations** - Complex nested append operations are now readable with clear documentation
- **Boundary testing** - Comprehensive test suite covering 50+ edge cases including:
  - Control characters, null bytes, Unicode, reserved Windows names
  - Path traversal attempts, whitespace handling, filesystem limits
- **README security section** documenting:
  - File permission model (0600/0700)
  - Symlink attack protection
  - Atomic file operations
  - Input validation security measures
  - SHA-256 content addressing
  - Security best practices

### Changed
- **Hash algorithm upgraded** from MD5 to SHA-256 for backup content addressing
  - Eliminates collision risk
  - Provides better security guarantees
  - Future-proof cryptographic hashing
- **Backup semantics documented** - Comprehensive documentation explaining content-addressed deduplication strategy
- **Error messages improved** - All errors now include context with `fmt.Errorf("operation: %w", err)` pattern
- **Close() error handling** - Fixed resource leaks by properly capturing deferred close errors using named returns

### Security
- **CRITICAL**: File permissions hardened from world-readable (0644/0755) to owner-only (0600/0700)
  - Prevents unauthorized access to sensitive settings on multi-user systems
- **CRITICAL**: Added symlink attack protection using `LstatIfPossible()` validation
  - Prevents malicious symlinks from overwriting system files
- **CRITICAL**: Switched from MD5 to SHA-256 for backup hashing
  - Eliminates cryptographic collision vulnerabilities
- **Path traversal protection enhanced** with explicit null byte detection
- **Early validation** prevents invalid names from reaching file operations

### Fixed
- **CRITICAL**: Atomic file replacement - Removed dangerous `Remove()` call before rename
  - Unix `rename()` is already atomic and overwrites destination
  - Eliminates data loss window where settings could be permanently lost
- **Resource leak** - Fixed deferred `Close()` calls that ignored errors
  - File writes now properly check for buffer flush failures
- **Error wrapping consistency** - All error returns now include proper context

### Internal
- **Manager** now accepts an optional `*slog.Logger` parameter
  - Defaults to discard logger if nil is passed
  - Tests pass nil for clean test output
- **Test coverage maintained** at 80.7% (exceeds 80% threshold)
- **All tests passing** across cmd/ccs, internal/ccs, and internal/cli packages

## [Previous Releases]

See [GitHub Releases](https://github.com/OpenGG/claude-code-switch-settings/releases) for information about previous versions.
