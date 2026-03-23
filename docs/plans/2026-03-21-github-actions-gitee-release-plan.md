# GitHub Actions → Gitee Release Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 当 GitHub `tag`（`v*`）发布时，自动构建 Windows `Setup.exe`、macOS `DMG`，并在 Gitee 创建同名 Release、上传附件、更新 `update/update-manifest.json`。

**Architecture:** GitHub Actions 负责跨平台构建与产物收集；最后一个 publish job 使用 `GITEE_TOKEN` 调用 Gitee API（Release + attach_files + contents）完成发布与更新清单写回。

**Tech Stack:** GitHub Actions、PowerShell 7（`pwsh`）、Bash、Inno Setup（Windows）、`hdiutil`（macOS）、Gitee API v5。

---

### Task 1: 新增 Gitee 发布脚本（PowerShell）

**Files:**
- Create: `scripts/publish-gitee-release.ps1`

**Step 1: 写脚本骨架与参数解析**
- 约定参数：
  - `-Tag`
  - `-GiteeOwner`（默认 `rainboxup`）
  - `-GiteeRepo`（默认 `pinchbot`）
  - `-GiteeToken`（必填，用于 API）
  - `-WindowsZip`（必填）
  - `-WindowsSetup`（必填）
  - `-MacDmg`（必填）
  - `-Branch`（默认 `main`）
- 输出：打印关键 URL 与 release_id，便于排查

**Step 2: 实现 release “获取或创建”**
- 调用：
  - `GET /v5/repos/{owner}/{repo}/releases/tags/{tag}`
  - 404 时调用 `POST /v5/repos/{owner}/{repo}/releases` 创建
- 需要能处理：
  - tag 在 Gitee 不存在时：创建 release 时传 `target_commitish=<branch>`

**Step 3: 实现附件上传（幂等）**
- 调用：
  - `GET /v5/repos/{owner}/{repo}/releases/{release_id}/attach_files` 获取已存在附件列表
  - 缺失的再 `POST /v5/repos/{owner}/{repo}/releases/{release_id}/attach_files` 上传（`multipart/form-data`）
- 规则：同名文件已存在则跳过

**Step 4: 实现 update-manifest 写回**
- 计算 Windows ZIP sha256（小写）
- 组装 `update/update-manifest.json`（JSON 格式与 `Launcher/app-wails/update.go` 的 `UpdateManifest` 一致）
- 调用：
  - `GET /v5/repos/{owner}/{repo}/contents/update/update-manifest.json?ref=<branch>` 读取 `sha`
  - `PUT /v5/repos/{owner}/{repo}/contents/update/update-manifest.json` 更新（`content` 为 base64，带 `sha`）
  - 若 GET 404：改用 `POST /contents/{path}` 新建

**Step 5: 快速本地自检（不依赖外网）**
- 运行：`pwsh -NoProfile -File scripts/publish-gitee-release.ps1 -?`
- 期望：能输出帮助/参数，不报语法错误

**Step 6: 提交**
```bash
git add scripts/publish-gitee-release.ps1
git commit -m "ci: add gitee release publish script"
```

---

### Task 2: 新增 GitHub Actions 工作流

**Files:**
- Create: `.github/workflows/release-gitee.yml`

**Step 1: build-windows**
- runner：`windows-latest`
- 安装 Inno Setup：`choco install innosetup`
- 构建命令：
  - `pwsh ./scripts/build-release.ps1 -Version $tag -Zip -Installer`
- artifact：上传 `dist/*.zip` 与 `dist/*-Setup.exe`

**Step 2: build-macos**
- runner：`macos-latest`
- 构建命令：
  - `bash ./scripts/build-release.sh "$tag"`
  - `bash ./scripts/package-dmg.sh "dist/PinchBot-$tag-Darwin-<arch>"`
- artifact：上传 `dist/*.dmg`

**Step 3: publish-gitee**
- runner：`ubuntu-latest`
- needs：`build-windows`、`build-macos`
- 下载 artifacts 到本地目录（例如 `artifacts/`）
- 调用发布脚本：
  - `pwsh ./scripts/publish-gitee-release.ps1 -Tag $tag -GiteeToken ${{ secrets.GITEE_TOKEN }} -WindowsZip <...> -WindowsSetup <...> -MacDmg <...>`

**Step 4: 提交**
```bash
git add .github/workflows/release-gitee.yml
git commit -m "ci: build installers and publish to gitee on tag"
```

---

### Task 3: 验证与推送

**Step 1: 检查 staged 文件只包含本任务内容**
- 运行：`git diff --cached --name-only`
- 期望：只有 `.github/workflows/release-gitee.yml`、`scripts/publish-gitee-release.ps1` 与本计划/设计文档

**Step 2: 推送到 GitHub 与 Gitee**
```bash
git push origin main
git push gitee main
```

