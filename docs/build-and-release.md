# 构建与发布 — 打包给客户

本文说明如何从本仓库构建**统一发布包**，直接发给客户使用。

## 发布包内容

构建完成后，在 **`dist/`** 下会得到按版本与平台命名的文件夹，例如：

```
dist/
  OpenClaw-1.0.0-Windows-x86_64/
    launcher-chat.exe       # 主程序（双击运行，会自动拉起下面两个且不弹黑框）
    picoclaw-launcher.exe   # 配置服务（端口 18800，由主程序自动启动）
    picoclaw.exe            # 网关（端口 18790，由主程序自动启动）
    config/
      config.example.json   # 示例配置，可复制到用户目录或通过配置页创建
    workspace-example/      # 示例工作区（USER.md、AGENTS.md、skills 等），可整目录复制到用户目录
    README.txt              # 使用说明与目录说明
```

**客户机上的用户数据**（首次运行后创建，不在发件包内）：
- 配置：`%USERPROFILE%\.picoclaw\config.json`
- 认证：`%USERPROFILE%\.picoclaw\auth.json`
- 工作区：`%USERPROFILE%\.picoclaw\workspace\`（配置里 `agents.defaults.workspace` 默认即此路径）
- **聊天记录**：`workspace\sessions\`（按会话的 .json 文件，使用后自动生成）
- **记忆**：`workspace\memory\`（MEMORY.md 长期记忆 + YYYYMM\YYYYMMDD.md 每日笔记）
- 其它：`workspace\state\`、`workspace\usage.jsonl`、`workspace\cron\` 等随使用自动创建

**Workspace 的两种用法**：  
1）**直接复制目录**：把发件包里的 `workspace-example` 整个复制到 `%USERPROFILE%\.picoclaw\workspace\`，即得到与仓库一致的工作区结构（USER.md、AGENTS.md、skills 等）。  
2）**用命令行初始化**：在发件包目录打开终端执行 `picoclaw onboard`，会生成 config 并把**内嵌**的工作区模板写入 `%USERPROFILE%\.picoclaw\workspace\`（无需手动复制）。

将整个 **`OpenClaw-<版本>-Windows-x86_64`** 文件夹（或打成 ZIP）发给客户即可。

## 前置要求

- **Go**：1.22+（PicoClaw 与 Launcher 各自 go.mod 要求的版本）
- **Windows 包**：在 Windows 上执行 `build-release.ps1`；或在 Linux/macOS 上交叉编译时使用脚本内的 `GOOS=windows GOARCH=amd64`。
- **macOS 包**：在 **Mac** 上执行 `build-release.sh`（需本机为 Darwin，以生成 darwin/arm64 或 amd64）。

## 一键构建（推荐）

在仓库根目录执行：

```powershell
.\scripts\build-release.ps1
```

- 版本号默认由 `git describe --tags --always --dirty` 得到，也可传入 `-Version "1.0.0"`。
- 输出目录：`dist\OpenClaw-<版本>-Windows-x86_64\`。

同时生成 ZIP（便于发送）：

```powershell
.\scripts\build-release.ps1 -Zip
```

指定版本号并打 ZIP：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Zip
```

### macOS 一键构建

在 **Mac** 上、仓库根目录执行：

```bash
chmod +x scripts/build-release.sh
./scripts/build-release.sh
```

- 版本号默认来自 `git describe`，也可传入第一个参数：`./scripts/build-release.sh 1.0.0`
- 输出目录：`dist/OpenClaw-<版本>-Darwin-arm64/`（Apple Silicon）或 `Darwin-amd64`（Intel）
- 打压缩包：`./scripts/build-release.sh -z` 或 `./scripts/build-release.sh 1.0.0 -z`，会在 `dist/` 下生成 `.tar.gz`

产物为可执行文件 `picoclaw`、`picoclaw-launcher`、`launcher-chat`（无 .exe），以及 `config/`、`workspace-example/`、`README.txt`。用户数据路径为 `~/.picoclaw/`。

## 构建步骤说明

脚本会依次：

1. **PicoClaw**
   - 在 `PicoClaw/` 下执行 `go generate ./...`（若需嵌入 workspace 等）。
   - 使用 `GOOS=windows GOARCH=amd64` 编译：
     - `picoclaw.exe`（`cmd/picoclaw`）
     - `picoclaw-launcher.exe`（`cmd/picoclaw-launcher`）
2. **Launcher 聊天小窗**
   - 在 `Launcher/app-wails/` 下用 Wails 生产构建：
     - `go build -tags "desktop,production" -ldflags "-H windowsgui" -o launcher-chat.exe .`
   - 得到 `launcher-chat.exe` 并放入同一发布目录。
3. **使用说明**
   - 在发布目录中生成 `使用说明.txt`，说明三个 exe 的用途与推荐使用顺序。

客户只需解压（若为 ZIP）后按「使用说明.txt」操作即可。

## 使用 GoReleaser（可选）

若希望做多平台发布、生成 tar.gz/zip 及 deb/rpm 等，可使用 PicoClaw 自带的 GoReleaser 配置：

- 配置位于：`PicoClaw/.goreleaser.yaml`
- 在 **PicoClaw** 目录下执行：`goreleaser release --snapshot`（测试）或打 tag 后 `goreleaser release`

注意：GoReleaser 当前只构建 **PicoClaw** 内的三个二进制（picoclaw、picoclaw-launcher、picoclaw-launcher-tui），**不包含** Launcher 的 `launcher-chat.exe`。若需要「一个包内同时包含 gateway + 配置页 + 聊天小窗」，请以 **`scripts/build-release.ps1`** 为准，或自行在 CI 中先跑该脚本，再将生成的 `dist/` 目录或 ZIP 作为发布产物。

## 目录约定

| 路径 | 说明 |
|------|------|
| `dist/` | 统一发布输出目录，已加入 `.gitignore`，不提交 |
| `scripts/build-release.ps1` | Windows 一键构建脚本 |
| `scripts/build-release.sh` | macOS 一键构建脚本（需在 Mac 上执行） |
| `PicoClaw/build/` | PicoClaw 自身 Makefile 的构建输出，不用于最终发客户 |

客户交付物：Windows 为 **`dist/OpenClaw-<版本>-Windows-x86_64`**（或其 ZIP）；macOS 为 **`dist/OpenClaw-<版本>-Darwin-arm64`**（或 amd64）或其 `.tar.gz`，无需再包含仓库或源码。
