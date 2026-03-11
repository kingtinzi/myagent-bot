# OpenClaw release build - produces a folder (or ZIP) to ship to customers.
# Usage: .\scripts\build-release.ps1 [-Version "1.0.0"] [-Zip]
# Output: dist\OpenClaw-<version>-Windows-x86_64\ (exe files + README)

param(
    [string]$Version = "",
    [switch]$Zip
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$DistDir = Join-Path $RepoRoot "dist"
$PicoClawDir = Join-Path $RepoRoot "PicoClaw"
$LauncherWailsDir = Join-Path (Join-Path $RepoRoot "Launcher") "app-wails"

if (-not $Version) {
    try {
        $Version = & git -C $RepoRoot describe --tags --always --dirty 2>$null
        if (-not $Version) { $Version = "dev" }
    } catch {
        $Version = "dev"
    }
}

$Platform = "Windows-x86_64"
$PackageName = "OpenClaw-$Version-$Platform"
$OutDir = Join-Path $DistDir $PackageName
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "  OpenClaw Release Build" -ForegroundColor Cyan
Write-Host "  Version: $Version  Output: $OutDir" -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan

# 1. Build PicoClaw (gateway + launcher)
Write-Host "`n[1/3] Building PicoClaw (picoclaw + picoclaw-launcher) ..." -ForegroundColor Yellow
Push-Location $PicoClawDir
try {
    go generate ./...
    if (-not $?) { Write-Host "  go generate warning (ok to ignore)" -ForegroundColor DarkYellow }

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    go build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "picoclaw.exe") ./cmd/picoclaw
    if (-not $?) { throw "picoclaw build failed" }

    go build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "picoclaw-launcher.exe") ./cmd/picoclaw-launcher
    if (-not $?) { throw "picoclaw-launcher build failed" }
} finally {
    Pop-Location
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
}

# 2. Build Launcher (Wails tray + chat window)
Write-Host "`n[2/3] Building Launcher (launcher-chat.exe) ..." -ForegroundColor Yellow
Push-Location $LauncherWailsDir
try {
    go build -tags "desktop,production" -ldflags "-s -w -H windowsgui -X main.Version=$Version" -o (Join-Path $OutDir "launcher-chat.exe") .
    if (-not $?) { throw "launcher-chat build failed" }
} finally {
    Pop-Location
}

# 3. Copy config example, workspace example, and write README
Write-Host "`n[3/3] Copying config + workspace example and writing README ..." -ForegroundColor Yellow
$ConfigExampleSrc = Join-Path (Join-Path $PicoClawDir "config") "config.example.json"
$ConfigDir = Join-Path $OutDir "config"
$ConfigExampleDst = Join-Path $ConfigDir "config.example.json"
if (Test-Path $ConfigExampleSrc) {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    Copy-Item -Path $ConfigExampleSrc -Destination $ConfigExampleDst -Force
} else {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
}

$WorkspaceSrc = Join-Path $PicoClawDir "workspace"
$WorkspaceExampleDst = Join-Path $OutDir "workspace-example"
if (Test-Path $WorkspaceSrc) {
    if (Test-Path $WorkspaceExampleDst) { Remove-Item -Recurse -Force $WorkspaceExampleDst }
    Copy-Item -Path $WorkspaceSrc -Destination $WorkspaceExampleDst -Recurse -Force
}

$ReadmePath = Join-Path $OutDir "README.txt"
$ReadmeContent = @"
OpenClaw / PicoClaw - Usage
========================================
Version: $Version
Platform: $Platform

FOLDER STRUCTURE (what you are shipping)
----------------------------------------
  launcher-chat.exe       Main program (double-click this)
  picoclaw-launcher.exe   Config UI (auto-started, do not delete)
  picoclaw.exe            Gateway (auto-started, do not delete)
  config\
    config.example.json   Example config
  workspace-example\      Example workspace (USER.md, AGENTS.md, skills, etc.)
  README.txt              This file

USER DATA (on the customer PC - created at first run or by you)
---------------------------------------------------------------
  Config:     %USERPROFILE%\.picoclaw\config.json
  Auth:       %USERPROFILE%\.picoclaw\auth.json
  Workspace:  %USERPROFILE%\.picoclaw\workspace\

  Under workspace (created automatically when the customer uses the app):
  - Chat history:  workspace\sessions\   (.json per conversation)
  - Memory:        workspace\memory\    (MEMORY.md + YYYYMM\YYYYMMDD.md daily notes)
  - State/usage:   workspace\state\, workspace\usage.jsonl, workspace\cron\, etc.

The app reads the above paths from config (agents.defaults.workspace defaults to
%USERPROFILE%\.picoclaw\workspace). You can:

  - Config: Copy config\config.example.json to %USERPROFILE%\.picoclaw\config.json
    and edit, or use Settings (http://localhost:18800) to create it in the browser.

  - Workspace: Either copy the whole folder workspace-example to
    %USERPROFILE%\.picoclaw\workspace\ (so that directory contains USER.md, AGENTS.md,
    skills\, etc.), or run in a terminal: picoclaw onboard
    (this creates config + copies the built-in workspace template into .picoclaw\workspace).

MAIN PROGRAM: launcher-chat.exe
  Double-click to run. It auto-starts the config service and gateway (no extra windows).
  Tray icon: open chat window; Settings opens http://localhost:18800

"@
$ReadmeContent | Set-Content -Path $ReadmePath -Encoding UTF8

Write-Host "`nBuild done: $OutDir" -ForegroundColor Green
Get-ChildItem $OutDir | ForEach-Object { Write-Host "  - $($_.Name)" }

if ($Zip) {
    $ZipPath = Join-Path $DistDir "$PackageName.zip"
    Write-Host "`nCreating ZIP: $ZipPath" -ForegroundColor Yellow
    Compress-Archive -Path $OutDir -DestinationPath $ZipPath -Force
    Write-Host "ZIP created: $ZipPath" -ForegroundColor Green
}

Write-Host "`nShip the folder '$PackageName' or the ZIP to customers.`n" -ForegroundColor Cyan
