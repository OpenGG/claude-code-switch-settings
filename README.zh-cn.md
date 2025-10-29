# Claude Code Switcher (ccs)

[English](README.md)

Claude Code Switcher（`ccs`）是一款用于管理 Claude Code 多份 `settings.json` 配置的命令行工具。它维护一个具名设置仓库、一个追踪当前激活配置的状态文件以及一个采用 MD5 校验和的备份目录，确保切换和覆盖操作安全可靠。

## 安装

1. 从 [GitHub Releases](https://github.com/OpenGG/claude-code-switch-settings/releases) 下载最新的发布包。
2. 解压后将 `ccs` 可执行文件放入 `PATH` 路径（例如 `~/bin`）。
3. 运行 `ccs --help` 验证安装是否成功。

> **注意：** 当向 `main` 分支推送符合 SemVer 规范的标签（例如 `v1.0.0`）时，将触发 GitHub Actions 自动构建并发布 macOS 二进制文件。

## 使用方法

所有文件均位于 `~/.claude/` 目录下。

### `ccs list`

```
ccs list
```

列出 `~/.claude/switch-settings/` 内的所有设置。当前激活的设置前缀为 `*`，内容发生变化的激活设置会标记 `(active, modified)`，缺失的激活设置显示 `(active, missing!)`，而尚未保存的当前配置会提示 `* (Current settings.json is unsaved)`。

### `ccs use`

```
ccs use <name>
```

将 `<name>.json` 从设置仓库复制到 `~/.claude/settings.json`，在覆盖前备份原文件，并把激活名称写入 `settings.json.active`。若未提供名称，将弹出交互式选单。

### `ccs save`

```
ccs save
```

把当前的 `settings.json` 保存到设置仓库，可新建或覆盖已有设置（覆盖前需确认），保存完成后即刻激活。命名校验确保新名称兼容 POSIX 与 Windows 文件系统。

### `ccs prune-backups`

```
ccs prune-backups --older-than 30d [--force]
```

清理 `~/.claude/switch-settings-backup/` 中超过指定时长未更新的备份。如果未提供 `--older-than`，程序会提供 30、90、180 天等常用选项供选择。

## 备份机制

在执行 `ccs use` 或 `ccs save` 的覆盖操作前，原文件会复制到 `~/.claude/switch-settings-backup/`，文件名为内容的 MD5 校验值。若备份已存在，将更新其修改时间以记录最近一次备份。

## 贡献指南

1. 安装 Go 1.21 或更高版本。
2. 在提交前运行 `gofmt -s -w` 保持代码风格一致。
3. 执行以下质量与测试流程：
   - `gofmt -l .`
   - `go vet ./...`
   - `go test -coverprofile=coverage.out ./...`
   - `go tool cover -func=coverage.out`（总覆盖率需 **>= 80%**）
4. 提交 PR 时请提供清晰的变更说明。向 `main` 分支推送 SemVer 标签（如 `v1.0.0`）将触发自动发布。

## 许可协议

本项目基于 MIT 许可协议发布，详见 [LICENSE](LICENSE)。
