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
$PinchBotDir = Join-Path $RepoRoot "PinchBot"
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

# 1. Build PinchBot (gateway + launcher)
Write-Host "`n[1/3] Building PinchBot (PinchBot + PinchBot-launcher) ..." -ForegroundColor Yellow
Push-Location $PinchBotDir
try {
    go generate ./...
    if (-not $?) { Write-Host "  go generate warning (ok to ignore)" -ForegroundColor DarkYellow }

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    go build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "PinchBot.exe") ./cmd/picoclaw
    if (-not $?) { throw "PinchBot build failed" }

    go build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "PinchBot-launcher.exe") ./cmd/picoclaw-launcher
    if (-not $?) { throw "PinchBot-launcher build failed" }
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

# 3. README only (no config/workspace copy: 首次运行在程序目录自动生成 .pinchbot)
Write-Host "`n[3/3] Writing README ..." -ForegroundColor Yellow
$ReadmePath = Join-Path $OutDir "README.txt"
$ReadmeContent = @"
OpenClaw / PinchBot - Usage (Portable)
========================================
Version: $Version
Platform: $Platform

FOLDER STRUCTURE
----------------------------------------
  launcher-chat.exe       Main program (double-click this)
  PinchBot-launcher.exe   Config UI (auto-started)
  PinchBot.exe            Gateway (auto-started)
  .pinchbot\              Created on first run (config + workspace)
  README.txt              This file

On first run, the program creates .pinchbot in this directory (config.json and workspace).
If .pinchbot already exists, it is left unchanged. All data stays next to the exe for easy management.

MAIN PROGRAM: launcher-chat.exe
  Double-click to run. Tray: open chat; Settings opens http://localhost:18800

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
