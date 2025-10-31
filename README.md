# Claude Code Switcher (ccs)

[中文说明](README.zh-cn.md)

Claude Code Switcher (`ccs`) is a CLI utility that manages multiple `settings.json` profiles for Claude Code. The tool maintains a repository of named settings, a state file that tracks the active profile, and a content-addressed backup directory to protect user data.

## Installation

1. Download the latest release archive from [GitHub Releases](https://github.com/OpenGG/claude-code-switch-settings/releases).
2. Extract the archive and place the `ccs` binary somewhere on your `PATH` (for example `~/bin`).
3. Run `ccs --help` to confirm the installation.

> **Note:** Releases are created whenever a SemVer tag (e.g. `v1.0.0`) is pushed to the `main` branch. Each release contains a macOS binary built by GitHub Actions.

## Usage

All files live inside `~/.claude/`.

### `ccs list`

```
ccs list
```

Lists every stored settings profile inside `~/.claude/switch-settings/`. The active profile is prefixed with `*`, modified profiles show `(active, modified)`, missing profiles show `(active, missing!)`, and unsaved local settings are highlighted as `* (Current settings.json is unsaved)`.

### `ccs use`

```
ccs use <name>
```

Loads `<name>.json` from `~/.claude/switch-settings/` into `~/.claude/settings.json`, backs up the previous `settings.json`, and records the active profile name in `settings.json.active`. When the name is omitted, an interactive selector is displayed.

### `ccs save`

```
ccs save
```

Saves the current `settings.json` into the settings repository, creating a new profile or overwriting an existing one after confirmation. The saved profile becomes active. A name validator ensures compatibility with both POSIX and Windows file systems.

### `ccs prune-backups`

```
ccs prune-backups --older-than 30d [--force]
```

Deletes backups in `~/.claude/switch-settings-backup/` that have not been refreshed within the specified duration. Without `--older-than`, an interactive menu offers common retention windows such as 30, 90, or 180 days.

## How Backups Work

Before `ccs use` or `ccs save` overwrites any file, the previous contents are copied into `~/.claude/switch-settings-backup/` using a SHA-256 hash as the filename. If a backup with the same checksum already exists, its modification time is refreshed to capture the most recent backup event. Empty files are backed up with a warning logged.

## Security

### File Permissions

All settings files and directories are created with restrictive permissions to protect sensitive data:
- **Directories**: `0700` (owner read/write/execute only - `drwx------`)
- **Files**: `0600` (owner read/write only - `-rw-------`)

This prevents other users on multi-user systems from reading your settings, which may contain API keys, authentication tokens, workspace configurations, or other sensitive data.

### Symlink Protection

`ccs` validates that target paths are not symbolic links before performing file operations. This prevents symlink attacks where a malicious actor could create a symlink to a system file (e.g., `/etc/passwd`) and trick `ccs` into overwriting it.

### Atomic File Operations

All file replacements use atomic rename operations. If a `ccs use` or `ccs save` operation fails partway through, your existing settings remain intact. There is no window where settings files are partially written or missing.

### Input Validation

Settings profile names undergo comprehensive validation to prevent:
- **Path traversal attacks**: Names like `../../../etc/passwd` are rejected
- **Null byte injection**: Names containing null bytes (`\x00`) are rejected
- **Reserved filenames**: Windows reserved names (CON, PRN, AUX, etc.) are rejected
- **Invalid characters**: Filesystem-unsafe characters (`<>:"/\|?*`) are blocked
- **Non-ASCII characters**: Only printable ASCII (0x20-0x7E) is allowed

### Content Addressing

Backups use SHA-256 cryptographic hashing for content addressing, which:
- Eliminates collision risks present in MD5
- Ensures identical settings files share a single backup
- Prevents accidental data loss from hash collisions

### Best Practices

1. **Regular backups**: Use `ccs prune-backups` judiciously - keep at least 30 days of backups
2. **Permissions audit**: Verify `~/.claude/` permissions with `ls -la ~/.claude/`
3. **Multi-user systems**: On shared systems, ensure your home directory is not world-readable
4. **Sensitive data**: Consider encrypting `~/.claude/` if your settings contain highly sensitive data

## Contributing

1. Install Go 1.21 or later.
2. Run `gofmt -s -w` on all Go files before committing.
3. Execute the quality and test suite:
   - `gofmt -l .`
   - `go vet ./...`
   - `go test -coverprofile=coverage.out ./...`
   - `go tool cover -func=coverage.out` (total coverage must be **>= 80%**)
4. Submit pull requests with descriptive summaries. Releases are triggered by pushing a SemVer tag (e.g. `v1.0.0`) to the `main` branch.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
