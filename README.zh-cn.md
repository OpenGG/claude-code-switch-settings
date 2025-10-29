# Claude Code Switcher (ccs)

[English](README.md) | 简体中文

`ccs` 是面向 Claude Code 用户的命令行工具，用于安全地管理多个 `settings.json` 配置。该工具维护一个命名设置档案库、记录当前激活的设置，并在每次覆盖操作前执行可靠的备份策略。

## 安装

发布流程会在主分支推送 `v*` 标签时自动生成 macOS 预编译包。下载压缩包并将二进制文件放到 `$PATH` 中：

```bash
# 将 v1.0.0 替换为所需版本
curl -LO https://github.com/example/claude-code-switch-settings/releases/download/v1.0.0/ccs-macos-amd64.tar.gz
mkdir -p ~/bin
tar -xzf ccs-macos-amd64.tar.gz -C ~/bin
chmod +x ~/bin/ccs
```

也可以使用 Go 1.21 及以上版本从源码构建：

```bash
git clone https://github.com/example/claude-code-switch-settings.git
cd claude-code-switch-settings
go build -o ccs ./cmd/ccs
```

## 目录结构

默认情况下，`ccs` 管理 `~/.claude` 目录中的文件。通过设置环境变量 `CCS_HOME` 可切换到其他根目录（适合测试或 CI 环境）。

```
~/.claude/
├── settings.json             # Claude Code 当前读取的配置
├── settings.json.active      # 当前激活设置的名称
├── switch-settings/          # 命名设置档案库（例如 work.json）
└── switch-settings-backup/   # 基于 MD5 的去重备份目录
```

## 使用方法

所有命令都以交互式体验为主，动词含义清晰：

- `ccs list`：列出所有设置、突出显示激活项，并标记已修改或缺失的状态。
- `ccs use [name]`：加载档案库中的设置到 `settings.json`，自动备份旧文件并更新激活状态。
- `ccs save`：交互式地保存当前 `settings.json` 到档案库（可覆盖或新建）并激活。
- `ccs prune-backups [--older-than 30d] [--force]`：删除在阈值时间内未触碰的备份文件。除非使用 `--force`，否则命令会提示确认。

## 备份策略

`save` 与 `use` 命令在覆盖目标文件之前都会创建备份。备份按内容的 MD5 命名，重复内容只存储一次，但每次使用都会刷新修改时间，以便准确反映最近的使用记录。

## 贡献指南

1. 安装 Go 1.21 以上版本。
2. 提交前使用 `gofmt -w` 格式化代码。
3. 本地运行以下命令保证测试与覆盖率（总覆盖率需保持在 80% 以上）：
   ```bash
   go test ./...
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out
   ```
4. 提交 PR 时请使用英文描述，并在文档更新时同时维护 `README.md` 与 `README.zh-cn.md`。

## 许可证

项目使用 MIT 许可证，详见 [LICENSE](LICENSE)。
