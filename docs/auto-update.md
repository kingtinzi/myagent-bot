# 自动更新 — 技术方案

本文描述 OpenClaw 桌面端（launcher-chat + PinchBot 套件）的**自动更新**技术方案：用户打开产品时在后台检查并下载新版本，**下次启动**时使用最新版本，无需手动下载安装包。

## 目标行为

1. **后台检查**：主程序（launcher-chat）启动后，在后台请求「更新清单」接口，不阻塞界面。
2. **发现新版本**：若清单中的版本高于当前版本，在后台下载新版本包（ZIP）到本地临时目录。
3. **下次启动生效**：本次不替换正在运行的程序；退出时安排「下次启动前」执行替换逻辑，下次启动即为新版本。

这样实现可以避免在 Windows 上覆盖正在运行的 exe（会失败），且用户无感知：打开照常用，下次打开就是新版本。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│  发布方（你）                                                      │
│  • 构建：scripts/build-release.ps1 → dist/OpenClaw-<版本>-.../   │
│  • 将整目录打成 ZIP 上传到发布源（如 GitHub Releases / 自建 CDN）   │
│  • 提供「更新清单」JSON 的 URL（见下）                              │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│  更新清单 API（GET 返回 JSON）                                     │
│  • version: 当前发布版本号（如 "1.0.1"）                           │
│  • url: 完整 ZIP 下载地址                                          │
│  • zip_folder: ZIP 内顶层目录名（如 "OpenClaw-1.0.1-Windows-x86_64"）│
│  • release_date / notes 等可选                                    │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│  客户端（用户电脑）                                                │
│  1. launcher-chat 启动 → 注入版本号（构建时 -ldflags）              │
│  2. 后台 goroutine：GET 清单，比较 version，若更新则下载 ZIP        │
│  3. 下载到 %LOCALAPPDATA%\OpenClaw\pending\xxx.zip                 │
│  4. 退出时：生成「延迟更新脚本」，启动脚本后进程退出                 │
│  5. 脚本：等待进程结束 → 解压 ZIP 覆盖安装目录 → 启动 launcher-chat │
│  6. 下次用户打开 = 新版本                                          │
└─────────────────────────────────────────────────────────────────┘
```

## 一、版本号来源

- **当前版本**：构建时通过 `-ldflags "-X main.Version=xxx"` 注入到 launcher-chat，例如：
  - `scripts/build-release.ps1` 中已有 `$Version`（来自 `git describe --tags --always --dirty`），在构建 launcher-chat 时传入即可。
- **清单中的版本**：由你维护的「更新清单」JSON 中的 `version` 字段提供；建议与发布包目录名、ZIP 名一致（如 `1.0.1`）。

## 二、更新清单 API 约定

客户端会对一个固定 URL 做 **GET** 请求（可配置，默认占位符），期望返回 **JSON**：

```json
{
  "version": "1.0.1",
  "url": "https://example.com/releases/OpenClaw-1.0.1-Windows-x86_64.zip",
  "zip_folder": "OpenClaw-1.0.1-Windows-x86_64",
  "release_date": "2025-03-11",
  "notes": "可选，更新说明"
}
```

| 字段         | 必填 | 说明 |
|--------------|------|------|
| `version`    | 是   | 当前发布版本号，用于与本地比较 |
| `url`        | 是   | 完整 ZIP 下载地址（GET 可直下） |
| `zip_folder` | 是   | ZIP 内唯一顶层目录名，解压后把该目录下内容覆盖到安装目录 |
| `release_date` | 否  | 发布日期 |
| `notes`      | 否   | 更新说明，可展示给用户 |

若使用 **GitHub Releases**：  
可把上述 JSON 放在 Release 的 description 或附件说明中，或单独建一个 repo/raw 文件；客户端改为请求该 URL。也可以自建一个简单 HTTP 服务，根据平台返回对应 JSON。

## 三、客户端流程（Launcher 内实现）

1. **启动时**
   - 读取当前版本（编译期注入的 `Version`）。
   - 异步调用「检查更新」：GET 清单 URL，解析 JSON，比较 `version` 与当前版本（按语义化版本或字符串均可，建议统一用 semver 或 `git describe` 格式）。
   - 若不需更新：仅记录日志，不做后续动作。
   - 若需要更新：在后台下载 `url` 到 `%LOCALAPPDATA%\OpenClaw\pending\<文件名>.zip`，下载完成后写入「待应用」标记（如记录 zip 路径、zip_folder、安装目录）。

2. **退出时**
   - 若存在「待应用」标记：生成「延迟更新脚本」：
     - 脚本内容：等待当前进程完全退出（如 Sleep 几秒）→ 解压 pending ZIP 到临时目录 → 将 `zip_folder` 内所有文件覆盖到安装目录（即 launcher-chat.exe 所在目录）→ 删除 pending 文件与标记 → 启动安装目录下的 `launcher-chat.exe`。
   - 启动该脚本（如 PowerShell 或 cmd），并立即退出主进程。脚本在进程退出后执行，避免「覆盖正在运行的 exe」问题。

3. **下次启动**
   - 用户再次双击 launcher-chat：运行的已是覆盖后的新版本，无需额外步骤。

安装目录 = `filepath.Dir(os.Executable())`，与现有「同目录 PinchBot-launcher / PinchBot」逻辑一致。

## 四、安全与体验建议

- **HTTPS**：清单与下载 URL 均使用 HTTPS，避免中间人篡改。
- **校验（可选）**：清单中可增加 `sha256`，下载完成后校验再标记为待应用；未校验则仅依赖 HTTPS。
- **重试**：网络失败时静默重试或下次启动再检查，不打扰用户。
- **不打断使用**：不在本次会话内替换 exe，避免弹窗或强制重启；「下次启动即最新」即可。

## 五、配置项（可选）

可在 Launcher 或配置文件中增加：

- **更新清单 URL**：默认指向你的发布源；企业内网可改为内网地址。
- **是否启用自动更新**：例如 `auto_update: true/false`，便于企业关闭外网更新。

## 六、与现有构建的关系

- **build-release.ps1**：在构建 launcher-chat 时增加 `-ldflags "-X main.Version=$Version"`，其余不变；发布包仍为目录 + 可选 ZIP。
- **发布流程**：你照常跑脚本得到 `dist/OpenClaw-<版本>-Windows-x86_64`，打成 ZIP 上传；同时更新「更新清单」JSON 中的 `version`、`url`、`zip_folder` 即可。

按上述方案即可实现「打开时后台更新，下次启动就是最新」的自动更新能力。

## 七、已实现的接口（Launcher 暴露给前端）

在 Wails 中已绑定 `App`，前端可通过 `window.go.main.App` 调用：

| 方法 | 说明 |
|------|------|
| `GetVersion()` | 返回当前版本号（构建时注入） |
| `CheckForUpdates()` | 主动检查更新；若有新版本会后台下载，返回 `{ current, available?, url?, notes?, error? }` |
| `HasPendingUpdate()` | 是否已有下载好的更新（下次启动将应用） |

- **启动时**：已自动在后台执行一次检查，若有新版本会静默下载。
- **退出时**：若存在待应用更新且为 Windows，会启动「延迟更新脚本」再退出，下次启动即为新版本。

**配置清单 URL**：代码中默认 `DefaultManifestURL` 为占位；发布前改为你的实际地址，或通过环境变量 `OPENCLAW_UPDATE_MANIFEST_URL` 覆盖。
