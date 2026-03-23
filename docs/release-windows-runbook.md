# Windows 发布 Runbook

本文是给发布负责人用的 **Windows 实操清单**，目标是在真实 Windows 环境里把 PinchBot 做成可对外分发的安装包，并完成代码签名与干净环境验收。

## 1. 发布前准备

至少准备：

- Windows 10/11 发布机
- Go 工具链
- Node.js 18+ + npm（**构建** `plugin-host` / `extensions/lobster` 时需要；最终用户机器若只用 Go 能力可不装 Node，但启用 Node 插件则仍需 Node）
- Inno Setup 6（如需安装器）
- 可用的代码签名证书
- `signtool.exe`
- 一套可用的 **远端 Platform API** 或（内测）本地 `platform-server` 配置

平台与聊天相关说明（**与当前默认打包策略一致**）：

- **默认发布包不编译、不附带 `platform-server.exe`**（`build-release.ps1` 需加 `-BundlePlatformServer` 才会打入本地后台）。
- 交付「登录后聊天 / 官方模型 / 钱包」等能力时，应在安装目录旁 `config\platform.env`（或用户侧等价配置）中配置 **`PICOCLAW_PLATFORM_API_BASE_URL`** 等，指向**远端**平台服务。
- 若使用**本机** `platform-server`，需自行构建并放入包内或 PATH，并保证存在可用的 `config\platform.env`；详见 `Launcher/app-wails/README.md` 中的解析顺序说明。
- 用户数据默认写入程序同目录下的 `.openclaw/`。

## 2. 构建发布包

在仓库根目录执行（示例）：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Zip -Installer
```

常用参数说明：

- **`-BundlePlatformServer`**：额外编译并打包 `platform-server.exe`（默认**不加**，适合接远端平台 API）。
- **`-NpmRegistry "https://registry.npmmirror.com"`**（可选）：构建时强制使用指定 npm 源；脚本也会自动 `npm ping` 探测可用源。
- **`-IncludeLivePlatformConfig`**：内测时若本机已有 `Platform\config\platform.env`，可一并拷入 `dist\...\config\`（勿用于对外泄露密钥的发包）。

产物通常包括：

- `dist\PinchBot-<版本>-Windows-x86_64\`（内含 `launcher-chat.exe`、`pinchbot.exe`、`plugin-host/`、`extensions/lobster/` 等；**不含** Node 版 `extensions/graph-memory`，图记忆为 **Go 原生**）
- `dist\PinchBot-<版本>-Windows-x86_64.zip`
- `dist\PinchBot-<版本>-Windows-x86_64-Setup.exe`

安装器默认安装到：

```text
%LOCALAPPDATA%\Programs\PinchBot
```

如有需要，也可以在安装向导中改到其他**用户可写目录**；不建议安装到 `Program Files` 之类权限更严格的位置。

## 3. 准备 live 配置

至少准备：

- `config\platform.env`（推荐：配置远端 **`PICOCLAW_PLATFORM_API_BASE_URL`**）

如果需要预置官方模型目录/路由/价格：

- `config\runtime-config.json`

建议方式：

1. 从 `config\platform.example.env` 复制出 `config\platform.env`
2. 填入真实 `PLATFORM_*` / 远端 API 地址
3. 需要预置官方模型时，再从 `runtime-config.example.json` 复制出 `runtime-config.json`

**Graph-memory（Go 原生）**：侧车文件示例见 `config\config.graph-memory.example.json`；用户侧可复制到 `.openclaw\config.graph-memory.json` 或通过环境变量指定路径（见包内 `README.txt`）。

## 4. 代码签名

推荐使用仓库内脚本：

```powershell
$env:WIN_SIGN_CERT_SHA1 = "你的证书指纹"
$env:WIN_SIGN_TIMESTAMP_URL = "http://timestamp.digicert.com"

.\scripts\sign-windows.ps1 `
  -PackageDir "dist\PinchBot-<版本>-Windows-x86_64" `
  -InstallerPath "dist\PinchBot-<版本>-Windows-x86_64-Setup.exe"
```

若包内**包含** `platform-server.exe`，签名命令中可对单文件追加参数（或脚本已支持目录内全部 exe 时一并签）。

脚本内部会调用：

- `signtool.exe sign`
- `signtool.exe verify`
- `Get-AuthenticodeSignature`

签名参数默认使用：

- 文件摘要：`sha256`
- 时间戳摘要：`sha256`
- RFC3161 时间戳：`/tr`

如果你的 `signtool.exe` 不在 PATH，可设置：

```powershell
$env:WIN_SIGNTOOL_PATH = "C:\Program Files (x86)\Windows Kits\10\bin\...\signtool.exe"
```

## 5. 签名后验证

对关键文件逐个确认（按你实际打入包内的二进制勾选）：

```powershell
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64\launcher-chat.exe"
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64\pinchbot.exe"
# 若打包了本地 platform 后端：
# Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64\platform-server.exe"
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64-Setup.exe"
```

要求：

- `Status = Valid`

同时也可以执行：

```powershell
signtool verify /pa /v "dist\PinchBot-<版本>-Windows-x86_64\launcher-chat.exe"
```

## 6. 本次迭代功能验收（程序已启动后）

以下为**当前版本已落地能力**的手动验收清单，适用于「安装包已安装 / 绿色包已解压，且已双击启动 `launcher-chat.exe`」的场景。可按优先级分批执行。

### 6.1 基础：启动与界面

| 序号 | 检查项 | 预期 |
|------|--------|------|
| B1 | 主窗口能打开，托盘图标正常 | 无立即崩溃；可再次打开主界面 |
| B2 | 设置页可打开（默认本机端口见包内 `README.txt`，常见为 `18800`） | 浏览器或内嵌页能加载设置 |
| B3 | 网关端口（默认 `18790`）未被防火墙策略误杀 | 本机日志无持续 bind 失败（如有） |

### 6.2 远端平台与登录（默认打包：无本机 `platform-server.exe`）

| 序号 | 检查项 | 预期 |
|------|--------|------|
| P1 | `config\platform.env` 已配置 **`PICOCLAW_PLATFORM_API_BASE_URL`** 指向可用远端 | 启动后不因平台不可达而无限白屏（允许短暂重试） |
| P2 | 登录流程 | 出现登录/鉴权相关 UI；登录成功后能进入聊天 |
| P3 | 官方模型列表 / 钱包或计费入口（若业务已开通） | 与远端环境一致，无 404 循环 |

若你**额外**打了 `-BundlePlatformServer` 且使用本机后端，再验证：存在 `platform.env` 时本机进程拉起与端口行为（见 `Launcher` README）。

### 6.3 Node 插件宿主与 Lobster

| 序号 | 检查项 | 预期 |
|------|--------|------|
| N1 | 配置中启用 Node 插件宿主（`plugins.node_host`）且需要 lobster 时 | `plugin-host/` 与 `extensions/lobster/` 存在且能加载 |
| N2 | **不应**再出现「Node 插件宿主」导致的**多余黑色 CMD 窗口**（需使用**已包含修复**的构建；旧包可能仍闪一下） | 无单独 `node.exe` 黑窗常驻 |
| N3 | Lobster 相关工具/工作流（若你启用了对应插件） | 调用路径可通，错误信息在日志中可查 |

### 6.4 Graph-memory（Go 原生，非 Node 扩展）

| 序号 | 检查项 | 预期 |
|------|--------|------|
| G1 | 配置中 `plugins.graph_memory_go_native` 为 **`true`**（默认） | 不依赖 `extensions/graph-memory` Node 包 |
| G2 | 侧车 `.openclaw\config.graph-memory.json`（从示例复制并填路径/密钥） | 启用后对话中图记忆工具可用（`gm_*`） |
| G3 | CLI 自检（可选，开发机） | 在 `PinchBot` 目录：`go run ./cmd/picoclaw check graph-memory` 能读库统计（库已存在时） |

### 6.5 回归与日志

| 序号 | 检查项 | 预期 |
|------|--------|------|
| R1 | 长时间空闲后再发消息 | 会话与网关仍可用 |
| R2 | 退出再启动 | `.openclaw` 配置与数据保留符合设计 |
| R3 | 出现问题时 | 能从日志中区分「平台 API」「网关」「Node 插件」「图记忆」错误来源 |

### 6.6 Feishu（openclaw-lark 插件优先）专项验收

当前版本行为（重要）：

- 当 `channels.feishu.enabled=true` 且 `plugins` 启用 `openclaw-lark` 时，**内置 Go Feishu 通道会自动跳过**。
- 如需紧急回退，仅用于调试：`channels.feishu.use_builtin=true`。

建议在 Windows 机器按下面检查：

| 序号 | 检查项 | 预期 |
|------|--------|------|
| F1 | `plugins.node_host=true`，且 `plugins.enabled` 或 `plugins.entries` 含 `openclaw-lark` | 启动日志不报 node host disabled |
| F2 | `channels.feishu` 配置了 `app_id/app_secret`（或 `appId/appSecret`） | 启动日志不报 credentials empty |
| F3 | `extensions/openclaw-lark/openclaw.plugin.json` 存在且合法 | 启动日志不报 plugin not discoverable |
| F4 | 访问 `GET /plugins/status` | `plugins_enabled` 中可见 `openclaw-lark` |
| F5 | 观察日志 | 出现“built-in Go channel skipped”说明插件优先生效 |
| F6 | 飞书端对话（私聊） | 能稳定收消息并回复 |
| F7 | 飞书群聊 @ 触发 | 仅在触发条件满足时回复，行为符合配置 |

---

**说明**：若验收失败，先确认：**远端平台地址**、**本机防火墙**、**config 是否与本次包内 `config.example.json` 字段一致**，以及是否使用了**含最新修复的安装包**（尤其是 Node 黑窗与 graph-memory 行为与构建日期相关）。

## 7. 干净 Windows 验收

建议使用一台 **干净 Windows** 虚拟机或未装开发环境的物理机，重点检查：

1. 从 ZIP 解压，或运行 `PinchBot-<版本>-Windows-x86_64-Setup.exe`
2. 如果走安装器，确认默认安装目录为 `%LOCALAPPDATA%\Programs\PinchBot`，或你手动选择的是其他用户可写目录
3. 双击 `launcher-chat.exe`
4. 确认 SmartScreen / Defender 不再因为未签名而弹出高风险拦截（签名后）
5. 按 **§6** 完成登录、设置、聊天与插件相关检查
6. 确认官方模型可见（依赖远端平台配置）
7. 确认钱包/充值协议可见（若业务已开通）
8. 如已配置支付，再确认支付跳转可用

同时检查实际进程与文件：

- `launcher-chat.exe`
- `pinchbot.exe`（若独立进程存在）
- `pinchbot-launcher.exe`（按需）
- **`platform-server.exe`**：仅当你打包了本地后端且使用本机模式时才应常驻；默认远端 API 场景下**可以不存在**

是否按预期拉起与退出。

## 8. 最终交付建议

推荐交付以下之一：

- 已签名 ZIP
- 已签名安装器 `PinchBot-<版本>-Windows-x86_64-Setup.exe`
- 已签名目录包（仅限内部或可信交付场景）

## 9. 发布前检查表

- [ ] Windows 发布包构建完成
- [ ] 远端 Platform API 或本机 `platform-server` 策略已明确（默认：**远端**）
- [ ] live `config\platform.env` 已准备（含 **`PICOCLAW_PLATFORM_API_BASE_URL`** 等，按环境填写）
- [ ] 如需预置，`config\runtime-config.json` 已准备
- [ ] `launcher-chat.exe` 已签名
- [ ] `pinchbot.exe` 已签名
- [ ] `pinchbot-launcher.exe` 已签名
- [ ] `platform-server.exe` 已签名（**仅当**打入包内时）
- [ ] 安装器 `PinchBot-<版本>-Windows-x86_64-Setup.exe` 已签名
- [ ] `Get-AuthenticodeSignature` 验证通过
- [ ] `signtool verify` 验证通过
- [ ] **§6 迭代功能验收**通过（至少 B、P、G 与你业务相关的 N 项）
- [ ] 干净 Windows 验收通过
- [ ] SmartScreen 行为已确认

## 10. 不要做的事

- 不要把 `platform.example.env` 当作 live 配置直接发客户
- 不要在未签名状态下对外宣称 Windows 包已可正式商用分发
- 不要跳过干净 Windows 验收
- 不要把内测用的 `-IncludeLivePlatformConfig` 产物含密钥直接外发
