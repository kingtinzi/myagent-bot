# GitHub Actions：构建 Windows Setup.exe / macOS DMG，并发布到 Gitee Release（设计）

日期：2026-03-21  
目标仓库：`rainboxup/pinchbot`（Gitee）  

## 目标与范围

当 GitHub 仓库打 `tag`（例如 `v1.2.3`）时，自动完成：

1) 在 GitHub Actions 上构建：
- Windows：`Setup.exe`（Inno Setup）与可用于自动更新的 `Windows ZIP`
- macOS：`DMG`（只读压缩镜像）

2) 自动在 Gitee 创建同名 `Release` 并上传附件（上述安装包/压缩包）

3) 自动更新 Gitee 仓库中的 `update/update-manifest.json`（用于 Windows 端自动更新）

不在本次范围内：
- Windows/macOS 代码签名、macOS notarization（后续可加）
- macOS 的“应用内自动更新”（目前更新流程主要面向 Windows ZIP 覆盖安装目录）

## 触发策略

- `on: push: tags: ['v*']`
- 保留 `workflow_dispatch` 便于手动补发（输入 tag 作为参数）

## 产物规范

### Windows
- 构建脚本：`scripts/build-release.ps1 -Version <tag> -Zip -Installer`
- 产物：
  - `dist/PinchBot-<tag>-Windows-x86_64.zip`
  - `dist/PinchBot-<tag>-Windows-x86_64-Setup.exe`（由 `scripts/windows-installer.iss` 产出）

### macOS
- 构建脚本：`scripts/build-release.sh <tag>`
- DMG 打包脚本：`scripts/package-dmg.sh <dist/PinchBot-<tag>-Darwin-<arch>>`
- 产物：
  - `dist/PinchBot-<tag>-Darwin-<arch>.dmg`

备注：macOS runner 只会产出其自身架构的包（`arm64` 或 `amd64`）。如需 Universal 或双架构，需要拆分为双 runner 构建 + lipo 合并（不在本次范围）。

## 发布到 Gitee（Release + 附件 + 更新清单）

### Release
- 获取：`GET /v5/repos/{owner}/{repo}/releases/tags/{tag}`
- 若不存在则创建：`POST /v5/repos/{owner}/{repo}/releases`
  - 参数（JSON）：`tag_name`、`name`、`body`、`target_commitish`、`prerelease`、`access_token`

### 附件上传
- 列表：`GET /v5/repos/{owner}/{repo}/releases/{release_id}/attach_files`
- 上传：`POST /v5/repos/{owner}/{repo}/releases/{release_id}/attach_files`
  - `multipart/form-data`，字段：`file`、`access_token`
- 去重策略：同名文件已存在则跳过上传（避免重复）

### 更新清单 update-manifest.json

文件：`update/update-manifest.json`（Gitee 仓库 `main` 分支）

更新字段（面向 Windows 自动更新 ZIP）：
- `version`: `<tag>`（原样保留 tag，例如 `v1.2.3`）
- `url`: `https://gitee.com/{owner}/{repo}/releases/download/<tag>/<WindowsZipFileName>`
- `zip_folder`: `PinchBot-<tag>-Windows-x86_64`（ZIP 内顶层目录名）
- `sha256`: 基于 Windows ZIP 计算的 SHA256（小写十六进制）
- `release_date`: `yyyy-MM-dd`
- `notes`: 可选（可写 GitHub release note 摘要，或留空）

更新 API：
- 先获取文件内容用于拿 `sha`：`GET /v5/repos/{owner}/{repo}/contents/update/update-manifest.json`
- 更新：`PUT /v5/repos/{owner}/{repo}/contents/update/update-manifest.json`
  - 参数（JSON）：`content(base64)`、`sha`、`message`、`branch`、`access_token`
- 若文件不存在则用 `POST /contents/{path}` 新建（兜底）

## GitHub Actions 设计

新增工作流：`.github/workflows/release-gitee.yml`

### build-windows（windows-latest）
- checkout（含 tags）
- 安装 Inno Setup（Chocolatey）
- 执行 `pwsh` 构建脚本
- 上传 artifacts：`PinchBot-*-Setup.exe`、`PinchBot-*.zip`

### build-macos（macos-latest）
- checkout
- 执行 `bash` 构建脚本产出 dist 目录
- 执行 `bash` DMG 脚本产出 `.dmg`
- 上传 artifact：`.dmg`

### publish-gitee（ubuntu-latest）
- 下载 artifacts
- `pwsh` 执行发布脚本：`scripts/publish-gitee-release.ps1`
- 依赖 GitHub Secrets：`GITEE_TOKEN`

## 安全与可维护性

- `GITEE_TOKEN` 仅在 publish job 使用
- 发布脚本支持 dry-run（可选）与幂等（重复跑不会重复上传同名附件）
- 错误处理：关键步骤失败则退出并打印可定位的错误信息（避免 silent failure）

