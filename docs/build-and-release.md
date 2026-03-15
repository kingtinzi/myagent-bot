# 构建与发布 — 打包给客户

本文说明如何从本仓库构建**统一发布包**。Windows 包可直接用于内部交付，也支持额外生成 Inno Setup 安装器；对外分发前仍建议补充代码签名。macOS 包会生成 `.app` 结构，面向客户分发前应完成签名与 notarization。

## 发布包内容

构建完成后，在 **`dist/`** 下会得到按版本与平台命名的文件夹，例如：

```
dist/
  PinchBot-1.0.0-Windows-x86_64/
    launcher-chat.exe       # 主程序（双击运行，自动拉起 gateway）
    pinchbot-launcher.exe   # 设置页服务（点击“设置”时按需启动）
    pinchbot.exe            # 网关（端口 18790）
    platform-server.exe     # 官方模型 / 钱包 / 充值后端（有 live 配置时自动启动）
    config/
      config.example.json
      platform.example.env
      runtime-config.example.json
    README.txt

  PinchBot-1.0.0-Darwin-arm64/
    launcher-chat.app/
      Contents/
        Info.plist
        MacOS/
          launcher-chat      # 主程序（Finder 双击打开）
          pinchbot-launcher  # 设置页服务（点击“设置”时按需启动）
          pinchbot           # 网关（端口 18790）
          platform-server    # 官方模型 / 钱包 / 充值后端（有 live 配置时自动启动）
    config/
      config.example.json
      platform.example.env
      runtime-config.example.json
    README.txt
```

**客户机上的用户数据**（首次运行后创建，不在发件包内）：
- 数据目录：**程序同目录** 的 `.pinchbot/`
- 配置：`.pinchbot/config.json`（首次运行自动创建，默认 `workspace: "workspace"`）
- 认证：`.pinchbot/auth.json`
- 工作区：`.pinchbot/workspace/`
- **聊天记录**：`workspace/sessions/`
- **记忆**：`workspace/memory/`
- 其它：`workspace/state/`、`workspace/usage.jsonl`、`workspace/cron/` 等随使用自动创建

首次运行自动创建 `.pinchbot`、默认 `config.json` 与工作区目录；PinchBot 在 Gateway 首次启动时会补齐 starter files。需要覆盖默认位置时，可使用 `PINCHBOT_HOME` / `PINCHBOT_CONFIG`。

Windows 包可先把 **`PinchBot-<版本>-Windows-x86_64`** 文件夹（或 ZIP）用于内部测试；若要正式外发，建议先补充代码签名。macOS 包建议在签名并完成 notarization 后，再把 **`PinchBot-<版本>-Darwin-<架构>`** 文件夹（或 `.tar.gz`）发给客户。

如果你要按正式 Windows 对外交付流程执行，请继续参考：

- `docs/release-windows-runbook.md`
- `scripts/sign-windows.ps1`

## 平台后端首跑说明

发布包里的平台后端相关文件现在都是 **example-only**：

- `config/platform.example.env`
- `config/runtime-config.example.json`

它们会随包分发，但**不会**在首跑时自动变成 live 配置，`platform-server` 默认也**不会**再读取 `platform.example.env`。

如果你确实要在客户机本地启用 `platform-server`，请显式执行：

1. 把 `config/platform.example.env` 复制为 `config/platform.env`
2. 按实际环境修改 `PLATFORM_*` 值（尤其数据库、Supabase、支付配置）
3. 如需示例官方模型配置，可再把 `config/runtime-config.example.json` 复制为 `config/runtime-config.json` 作为起点；如果不复制，服务会在首次 bootstrap 时创建一个空的 runtime config 文件
4. 再启动 `launcher-chat.exe`（或 macOS 下的 `launcher-chat.app`）；桌面入口会从发布包根目录自动拉起 `platform-server`，如需手动排障也可直接执行 `launcher-chat.app/Contents/MacOS/platform-server`

如果是在源码仓库里做本地联调，推荐先运行：

- macOS / Linux：`./scripts/bootstrap-local-platform-config.sh`
- Windows PowerShell：`.\\scripts\\bootstrap-local-platform-config.ps1`

它会把 example 文件复制成 live 文件，但不会替你填写真实密钥；复制后仍要把 `replace-with-your-upstream-api-key`、数据库、Supabase、支付参数替换掉。

注意：桌面聊天窗口本身就是先登录再使用的流程，因此只要你要交付 `launcher-chat` 给最终用户，`config/platform.env` 就不是“可选项”——没有它，登录后的聊天、官方模型、钱包、充值都无法工作。

这样可以避免“示例配置被误当成正式配置”导致的首跑失败。

## 前置要求

- **Go**：1.22+（PinchBot 与 Launcher 各自 go.mod 要求的版本）
- **Windows 包**：在 Windows 上执行 `build-release.ps1`；或在 Linux/macOS 上交叉编译时使用脚本内的 `GOOS=windows GOARCH=amd64`。
- **macOS 包**：在 **Mac** 上执行 `build-release.sh`（需本机为 Darwin，以生成 darwin/arm64 或 amd64）。

## 一键构建（推荐）

在仓库根目录执行：

```powershell
.\scripts\build-release.ps1
```

- 版本号默认由 `git describe --tags --always --dirty` 得到，也可传入 `-Version "1.0.0"`。
- 输出目录：`dist\PinchBot-<版本>-Windows-x86_64\`。

同时生成 ZIP（便于发送）：

```powershell
.\scripts\build-release.ps1 -Zip
```

指定版本号并打 ZIP：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Zip
```

生成 Windows 安装器（需本机安装 **Inno Setup 6** 或让 `ISCC.exe` 在 `PATH` 中）：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Installer
```

同时生成目录、ZIP 和安装器：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Zip -Installer
```

安装器会**固定安装到** **`%LOCALAPPDATA%\Programs\PinchBot`**（禁用目录选择页），这样应用仍然可以把 `.pinchbot/` 数据目录保留在“程序同目录”，同时避免 `Program Files` 写权限问题。

### macOS 一键构建

在 **Mac** 上、仓库根目录执行：

```bash
chmod +x scripts/build-release.sh
./scripts/build-release.sh
```

- 版本号默认来自 `git describe`，也可传入第一个参数：`./scripts/build-release.sh 1.0.0`
- 输出目录：`dist/PinchBot-<版本>-Darwin-arm64/`（Apple Silicon）或 `Darwin-amd64`（Intel）
- 打压缩包：`./scripts/build-release.sh -z` 或 `./scripts/build-release.sh 1.0.0 -z`，会在 `dist/` 下生成 `.tar.gz`
- 脚本每次会重建 `dist/PinchBot-<版本>-Darwin-<架构>/`，避免旧产物混入新的包
- 如需代码签名，可在执行前设置 `MAC_CODESIGN_IDENTITY="Developer ID Application: ..."`；脚本会对 `.app` 内的二进制和 bundle 执行 `codesign`
- 如需继续自动化 notarization，可执行：`./scripts/notarize-macos.sh dist/PinchBot-<版本>-Darwin-<架构> --keychain-profile <profile>`
- 如需生成最终 `.dmg` 交付物，可执行：`./scripts/package-macos-dmg.sh dist/PinchBot-<版本>-Darwin-<架构> --overwrite`

如果你要按正式对外交付流程执行，请继续参考：`docs/release-macos-runbook.md`

如果你要先做一次“登录 + 官方模型 + 钱包 + 充值协议”的本地完整联调，请同时参考：`docs/official-model-local-smoke-test.md`

macOS 发布目录现在会包含 `launcher-chat.app`，其内部 `Contents/MacOS/` 会同时放置 `launcher-chat`、`pinchbot`、`pinchbot-launcher`、`platform-server`，从而更接近可分发的 Finder 应用包结构；根目录保留 `config/` 与 `README.txt`。用户数据路径为程序同目录下的 `.pinchbot/`。其中 `platform.example.env` 和 `runtime-config.example.json` 仅作为示例，不会自动生效；只有在你显式创建 `config/platform.env` 后，桌面入口才应自动拉起 `platform-server` 并支持登录后的聊天、官方模型、钱包、充值功能。点击“设置”时按需启动 `pinchbot-launcher`，不会在首跑时常驻。若未设置 `MAC_CODESIGN_IDENTITY`，该 `.app` 默认是未签名包，客户侧可能被 Gatekeeper 拦截；正式外发前还应执行 Apple notarization。脚本会把 `git describe` 风格的版本号清洗成 Apple 可接受的 bundle version 字符串，再写入 `Info.plist`，同时把同一版本注入 `launcher-chat` 可见版本号与更新检查逻辑。

## 构建步骤说明

发布脚本会依次：

1. **构建 PinchBot 侧二进制**
   - 直接编译 `pinchbot` 与 `pinchbot-launcher`
   - 发布脚本**不会**在构建时执行 `go generate ./...`；工作区 starter templates 现在直接从 `internal/workspacetpl/workspace/` 嵌入，避免构建时回写源码树
2. **构建平台后端**
   - 在 `Platform/` 下编译 `platform-server`
3. **构建桌面聊天入口**
   - 在 `Launcher/app-wails/` 下构建 `launcher-chat`
   - Windows 包输出 `launcher-chat.exe`
   - macOS 包会把 `launcher-chat` 放进 `launcher-chat.app/Contents/MacOS/`
4. **拷贝示例配置**
   - 复制 `config.example.json`
   - 复制 `platform.example.env`、`runtime-config.example.json`
5. **生成发布说明**
   - 在发布目录中生成 `README.txt`
   - macOS README 会明确标记当前 bundle 是否已签名，并提醒外发前仍需 notarization
6. **可选：生成 Windows 安装器**
   - 使用 `scripts/windows-installer.iss`
   - 由 `build-release.ps1 -Installer` 调用 Inno Setup `ISCC.exe`
   - 安装路径固定为 `%LOCALAPPDATA%\Programs\PinchBot`
   - 安装器版本号与输出文件名会在脚本里做安全化处理，避免原始 Git tag / describe 字符串中的 `/` 等字符破坏构建
   - 默认生成 `dist/PinchBot-<版本>-Windows-x86_64-Setup.exe`

Windows 包解压后可按 `README.txt` 使用，但正式外发仍建议先做代码签名；macOS 包请先完成签名 / notarization，再按 `README.txt` 交付。

## 使用 GoReleaser（可选）

若希望做多平台发布、生成 tar.gz/zip 及 deb/rpm 等，可使用 PinchBot 自带的 GoReleaser 配置：

- 配置位于：`PinchBot/.goreleaser.yaml`
- 在 **PinchBot** 目录下执行：`goreleaser release --snapshot`（测试）或打 tag 后 `goreleaser release`

注意：GoReleaser 当前只构建 **PinchBot** 内的三个二进制（源码 module 已切到 `github.com/sipeed/pinchbot`），**不包含** Launcher 的桌面前端（Windows 的 `launcher-chat.exe` / macOS 的 `launcher-chat.app`）。若需要「一个包内同时包含 gateway + 配置页 + 聊天小窗」，请以 **`scripts/build-release.ps1`** 或 **`scripts/build-release.sh`** 为准，或自行在 CI 中先跑对应脚本，再将生成的 `dist/` 目录或压缩包作为发布产物。

## 目录约定

| 路径 | 说明 |
|------|------|
| `dist/` | 统一发布输出目录，已加入 `.gitignore`，不提交 |
| `scripts/build-release.ps1` | Windows 一键构建脚本（目录 / ZIP / 可选安装器） |
| `scripts/windows-installer.iss` | Windows Inno Setup 安装器模板 |
| `scripts/build-release.sh` | macOS 一键构建脚本（需在 Mac 上执行） |
| `PinchBot/build/` | PinchBot 自身 Makefile 的构建输出，不用于最终发客户 |

客户交付物：Windows 为 **`dist/PinchBot-<版本>-Windows-x86_64`**（或其 ZIP）；macOS 为 **`dist/PinchBot-<版本>-Darwin-arm64`**（或 amd64）或其 `.tar.gz`，无需再包含仓库或源码。
