# Claude Code Switcher (ccs)

[English](README.md)

`ccs` 是一款使用 Go 实现的命令行工具，用于管理多个 `Claude Code` 的 `settings.json` 配置。它维护命名设置库，追踪当前激活的配置，并在每次覆盖文件时保存按内容哈希的备份。

## 安装

### 从 GitHub Releases 下载

1. 在项目的 Releases 页面下载最新的 `ccs-macos-amd64.tar.gz` 压缩包。
2. 解压缩后将 `ccs` 可执行文件移动到你的 `$PATH` 目录（例如 `~/bin`）。
3. 确认文件具有执行权限：`chmod +x ~/bin/ccs`。

### 从源码构建

```bash
git clone https://github.com/example/claude-code-switch-settings.git
cd claude-code-switch-settings
go build -o ccs ./cmd/ccs
```

生成的 `ccs` 二进制文件可以放到任何 `$PATH` 目录中。

## 使用说明

所有命令默认操作 `~/.claude` 目录。可以通过设置 `CCS_BASE_DIR` 来覆盖默认位置（例如在自动化测试中使用临时目录）。

| 命令 | 说明 |
| --- | --- |
| `ccs list` | 列出所有已保存的设置，并标记当前激活或缺失的配置。 |
| `ccs use [name]` | 将指定名称的配置写入 `settings.json` 并设为激活状态；如果未提供名称，将显示交互式选择菜单。 |
| `ccs save` | 将当前 `settings.json` 保存到档案库并设为激活；可以覆盖现有设置或创建新设置。 |
| `ccs prune-backups [--older-than 30d] [--force]` | 清理早于指定阈值的备份文件；除非使用 `--force`，否则会进行确认提示。 |

### 交互辅助

- 设置 `CCS_NON_INTERACTIVE=1` 可关闭交互提示，并使用环境变量驱动命令（例如 `CCS_SELECT_SETTING`、`CCS_NEW_SETTINGS_NAME`、`CCS_PRUNE_DURATION`）。
- 通过 `CCS_BASE_DIR` 指向临时目录，可以安全地进行实验而不影响真实配置。

### 示例流程

```bash
# 将当前设置保存为 "work"
ccs save
# 切换到 "personal" 配置
ccs use personal
# 查看所有设置及状态
ccs list
# 无需确认，清理 45 天前的备份
ccs prune-backups --older-than 45d --force
```

## 备份机制

在执行 `save` 或 `use` 并覆盖任何文件之前，`ccs` 会计算旧文件的 MD5，并将其保存到 `~/.claude/switch-settings-backup/<md5>.json`。如果再次备份相同内容，只会刷新备份文件的修改时间，以便基于最近使用情况进行清理。通过 `ccs prune-backups` 可以安全删除超出保留期的备份文件。

## 参与贡献

1. 提交前使用 `gofmt -s` 格式化代码。
2. 运行完整测试并确保测试覆盖率始终 **不低于 80%**：
   ```bash
   go test ./...
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out
   ```
3. 参考 `.github/workflows/release.yml` 中的 CI 配置，确保改动不会降低覆盖率或破坏发布流程。

欢迎提交 Issue 和 Pull Request，请清晰描述改动并尽可能附带测试案例。
