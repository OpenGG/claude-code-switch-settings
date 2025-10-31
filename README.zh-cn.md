# Claude Code Switcher (ccs)

[English](README.md)

Claude Code Switcher（`ccs`）是一款用于管理 Claude Code 多份 `settings.json` 配置的命令行工具。它维护一个具名设置仓库、追踪当前激活配置的状态文件，并在任何破坏性操作前创建基于内容寻址的备份。

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

列出 `~/.claude/switch-settings/` 内的所有已保存配置。当前激活的配置前缀为 `*`，已修改的配置显示 `(active, modified)`，缺失的配置显示 `(active, missing!)`，尚未保存的本地设置会提示 `* (Current settings.json is unsaved)`。

### `ccs use`

```
ccs use <name>
```

从 `~/.claude/switch-settings/` 加载 `<name>.json` 到 `~/.claude/settings.json`，备份之前的 `settings.json`，并在 `settings.json.active` 中记录激活的配置名称。如果未提供名称，将显示交互式选择菜单。

### `ccs save`

```
ccs save
```

将当前的 `settings.json` 保存到设置仓库，可以创建新配置或在确认后覆盖已有配置。保存后该配置将成为激活状态。名称验证器会确保与 POSIX 和 Windows 文件系统兼容。

### `ccs prune-backups`

```
ccs prune-backups --older-than 30d [--force]
```

删除 `~/.claude/switch-settings-backup/` 中在指定时长内未被刷新的备份。如果未提供 `--older-than` 参数，会显示交互式菜单提供常用的保留时间选项，如 30、90 或 180 天。

## 备份机制

在 `ccs use` 或 `ccs save` 覆盖任何文件之前，之前的内容会使用 SHA-256 哈希值作为文件名复制到 `~/.claude/switch-settings-backup/`。如果相同校验和的备份已存在，则只更新其修改时间以记录最近的备份事件。空文件会被备份并记录警告日志。

## 安全性

### 文件权限

所有设置文件和目录都使用严格的权限创建以保护敏感数据：
- **目录**：`0700`（仅所有者可读/写/执行 - `drwx------`）
- **文件**：`0600`（仅所有者可读/写 - `-rw-------`）

这可以防止多用户系统上的其他用户读取您的设置，这些设置可能包含 API 密钥、身份验证令牌、工作区配置或其他敏感数据。

### 符号链接保护

`ccs` 在执行文件操作之前会验证目标路径不是符号链接。这可以防止符号链接攻击，恶意行为者可能会创建指向系统文件（例如 `/etc/passwd`）的符号链接，并欺骗 `ccs` 覆盖它。

### 原子文件操作

所有文件替换都使用原子重命名操作。如果 `ccs use` 或 `ccs save` 操作中途失败，您现有的设置将保持完整。不存在设置文件部分写入或丢失的时间窗口。

### 输入验证

配置名称经过全面验证以防止：
- **路径遍历攻击**：类似 `../../../etc/passwd` 的名称会被拒绝
- **空字节注入**：包含空字节（`\x00`）的名称会被拒绝
- **保留文件名**：Windows 保留名称（CON、PRN、AUX 等）会被拒绝
- **非法字符**：文件系统不安全的字符（`<>:"/\|?*`）会被阻止
- **非 ASCII 字符**：仅允许可打印的 ASCII 字符（0x20-0x7E）

### 内容寻址

备份使用 SHA-256 加密哈希进行内容寻址，这可以：
- 消除 MD5 中存在的碰撞风险
- 确保相同的设置文件共享单个备份
- 防止因哈希碰撞导致的意外数据丢失

### 最佳实践

1. **定期备份**：谨慎使用 `ccs prune-backups` - 至少保留 30 天的备份
2. **权限审计**：使用 `ls -la ~/.claude/` 验证 `~/.claude/` 权限
3. **多用户系统**：在共享系统上，确保您的主目录不是所有人可读的
4. **敏感数据**：如果您的设置包含高度敏感的数据，请考虑加密 `~/.claude/`

## 贡献指南

1. 安装 Go 1.21 或更高版本
2. 克隆仓库并创建功能分支
3. 按照以下指南进行更改
4. 提交包含清晰描述的 Pull Request

### 代码质量标准（必需）

提交前请运行以下检查：

```bash
gofmt -s -w .      # 格式化所有 Go 文件
go vet ./...       # 静态分析（将警告视为阻塞问题）
go test ./...      # 运行所有测试
```

### 测试标准

我们优先考虑**测试质量而非覆盖率数字**。详见 [TESTING.md](TESTING.md) 获取完整指南。

**覆盖率检查：**
```bash
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/...
go tool cover -func=coverage.out | tail -1
```

**目标：约 80% 有意义的覆盖率**
- 80% 阈值是指导原则，而非硬性要求
- 安全关键代码（validator）需要 >90% 覆盖率
- 不要为了提高数字而编写无意义的测试
- 集成测试也计入覆盖率

**应该测试的内容：**
- ✅ 安全验证（攻击防护）
- ✅ 复杂业务逻辑（算法、状态机）
- ✅ 集成工作流（面向用户的操作）

**不应该测试的内容：**
- ❌ 简单包装器（已通过集成测试覆盖）
- ❌ 简单的 getter/setter
- ❌ 第三方库

详细的架构信息请参阅 [CLAUDE.md](CLAUDE.md)，测试理念请参阅 [TESTING.md](TESTING.md)。

### 发布流程

向 `main` 分支推送 SemVer 标签（例如 `v1.0.0`）将触发发布。CI 工作流将：
1. 运行代码质量检查（gofmt、go vet）
2. 运行所有测试并验证覆盖率
3. 构建 macOS 二进制文件
4. 创建 GitHub 发布

## 许可协议

本项目基于 MIT 许可协议发布，详见 [LICENSE](LICENSE)。
