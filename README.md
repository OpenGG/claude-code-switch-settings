# Claude Code Switcher (ccs)

[简体中文](README.zh-cn.md) | English

`ccs` is a command line assistant for Claude Code users who want to manage multiple `settings.json` profiles safely. The tool maintains an archive of named settings, tracks the active profile, and enforces robust backup policies before every destructive operation.

## Installation

Pre-built binaries are published on GitHub Releases when tags matching `v*` are pushed to the main branch. Download the latest macOS archive and place the binary on your `$PATH`:

```bash
# Replace v1.0.0 with the desired release tag
curl -LO https://github.com/example/claude-code-switch-settings/releases/download/v1.0.0/ccs-macos-amd64.tar.gz
mkdir -p ~/bin
tar -xzf ccs-macos-amd64.tar.gz -C ~/bin
chmod +x ~/bin/ccs
```

Alternatively, build from source with Go 1.21 or newer:

```bash
git clone https://github.com/example/claude-code-switch-settings.git
cd claude-code-switch-settings
go build -o ccs ./cmd/ccs
```

## Configuration Layout

By default `ccs` manages files inside `~/.claude`. Set the `CCS_HOME` environment variable to target a different root directory (useful for testing and CI).

```
~/.claude/
├── settings.json             # Active settings used by Claude Code
├── settings.json.active      # Name of the active settings profile
├── switch-settings/          # Archive of named profiles (e.g. work.json)
└── switch-settings-backup/   # Deduplicated backups stored by MD5 hash
```

## Usage

All commands are interactive by default and expose clear verbs:

- `ccs list` – Report stored settings, highlight the active profile, and flag modified or missing states.
- `ccs use [name]` – Load the named profile into `settings.json`, back up the previous file, and mark the profile active.
- `ccs save` – Interactively save the current `settings.json` into the archive (overwriting or creating as needed) and activate it.
- `ccs prune-backups [--older-than 30d] [--force]` – Delete backup files that have not been touched within the threshold. Confirmation prompts protect against accidental deletions unless `--force` is supplied.

## Backup Strategy

Every `save` or `use` command backs up the file that is about to be overwritten. Backups are stored by MD5 hash, so identical content is saved once but the modification time is refreshed on each use. This design keeps the backup directory space-efficient while still recording the most recent time a version was important.

## Contributing

1. Install Go 1.21+.
2. Format code with `gofmt -w` before committing.
3. Run linters and tests locally:
   ```bash
   go test ./...
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out
   ```
   The CI workflow fails if total coverage drops below 80%.
4. Submit pull requests in English and include updates to both `README.md` and `README.zh-cn.md` when documentation changes are required.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
