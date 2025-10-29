# Repository Guidelines

## Project Structure & Module Organization
Claude Code Switcher (`ccs`) centers on the CLI entrypoint in `cmd/ccs/main.go`, which wires commands to the internal packages. Profile persistence and path rules live in `internal/ccs`, while `internal/cli` deals with interactive prompts. The `internal/core`, `internal/interface`, and `internal/infrastructure` directories are reserved for future layering—add new files there only when separation is justified. Keep tests beside their packages (`*_test.go`), and funnel filesystem logic through `internal/ccs/paths.go` so everything continues to target the `~/.claude/` tree.

## Build, Test, and Development Commands
- `gofmt -l .` spots files needing format; run `gofmt -s -w` before commits.
- `go build ./cmd/ccs` creates the binary; `go run ./cmd/ccs --help` checks wiring.
- `go vet ./...` applies Go static analysis; treat warnings as blockers.
- `go test -coverprofile=coverage.out ./...` and `go tool cover -func=coverage.out` enforce the 80% bar.

## Coding Style & Naming Conventions
Follow Go defaults: tabs, gofmt ordering, and grouped imports. Command names stay short and imperative (`list`, `save`, `prune-backups`). File names mirror package roles (`manager.go`, `promptui_prompter.go`). Wrap filesystem work in helpers under `internal/ccs` and handle errors explicitly so the CLI surfaces actionable messages.

## Testing Guidelines
Tests use the standard `testing` package and sit next to the code (`manager_test.go`, `main_test.go`). Favor table-driven tests for profile permutations and fake filesystem layers instead of touching the real `~/.claude` directory. Run `go test ./...` locally on every change, keep coverage ≥80%, and add regression cases when adjusting command output or prompts.

## Commit & Pull Request Guidelines
Recent history favors concise, imperative commit subjects (“Refine backup creation flow”). Continue that voice, optionally scoping with a noun phrase. Pull requests should outline the behavior change, list validation commands, and link issues or release notes. Include screenshots only if they clarify user-facing text. Note any documentation updates required when commands or backup semantics shift.
