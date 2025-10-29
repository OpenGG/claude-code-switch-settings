# Claude Code Switcher (ccs)

[中文文档](README.zh-cn.md)

`ccs` is a Go-based command line utility that manages multiple `Claude Code` settings profiles. It keeps a library of named `settings.json` variants, tracks which profile is active, and maintains content-addressed backups whenever files are overwritten.

## Installation

### From GitHub Releases

1. Download the latest archive (`ccs-macos-amd64.tar.gz`) from the project releases page.
2. Extract the archive and move the `ccs` binary into a directory on your `$PATH` (for example `~/bin`).
3. Ensure the binary is executable: `chmod +x ~/bin/ccs`.

### Build from Source

```bash
git clone https://github.com/example/claude-code-switch-settings.git
cd claude-code-switch-settings
go build -o ccs ./cmd/ccs
```

The resulting `ccs` binary can be placed anywhere on your `$PATH`.

## Usage

All commands operate on `~/.claude` by default. Set `CCS_BASE_DIR` to override this location (useful in automated tests).

| Command | Description |
| --- | --- |
| `ccs list` | List saved settings and show which profile is active or missing. |
| `ccs use [name]` | Load the named profile into `settings.json` and mark it active. If no name is provided an interactive picker is shown. |
| `ccs save` | Save the current `settings.json` into the archive and activate it. You can overwrite an existing profile or create a new one. |
| `ccs prune-backups [--older-than 30d] [--force]` | Remove backup files older than the provided threshold. Prompts for confirmation unless `--force` is supplied. |

### Interactive helpers

- `CCS_NON_INTERACTIVE=1` disables prompts and drives commands with environment variables (e.g. `CCS_SELECT_SETTING`, `CCS_NEW_SETTINGS_NAME`, `CCS_PRUNE_DURATION`).
- `CCS_BASE_DIR` can be pointed at a temporary directory for safe experimentation.

### Example workflow

```bash
# Save the current settings as "work"
ccs save
# Switch to the "personal" profile
ccs use personal
# List stored profiles and current status
ccs list
# Remove backups older than 45 days without a prompt
ccs prune-backups --older-than 45d --force
```

## Backup Strategy

Before any `save` or `use` action overwrites a file, `ccs` computes the MD5 checksum of the existing contents and stores a copy in `~/.claude/switch-settings-backup/<md5>.json`. If the same content is backed up again its modification time is refreshed so that pruning decisions reflect recent usage. Backups are safe to delete with `ccs prune-backups` when they have not been touched within the chosen retention window.

## Contributing

1. Format code with `gofmt -s` before committing.
2. Run the full test suite and ensure coverage remains **at least 80%**:
   ```bash
   go test ./...
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out
   ```
3. Use the provided GitHub Actions workflow (`.github/workflows/release.yml`) as a reference for CI checks. Contributions should not lower coverage or break the release job.

Issues and pull requests are welcome—please describe changes clearly and include tests for new behavior wherever possible.
