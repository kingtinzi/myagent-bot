# PinchBot release build - produces a folder (or ZIP) to ship to customers.
# Usage: .\scripts\build-release.ps1 [-Version "1.0.0"] [-Zip]
# Output: dist\PinchBot-<version>-Windows-x86_64\ (exe files + README)

param(
    [string]$Version = "",
    [switch]$Zip
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$DistDir = Join-Path $RepoRoot "dist"
$PinchBotDir = Join-Path $RepoRoot "PinchBot"
$LauncherWailsDir = Join-Path (Join-Path $RepoRoot "Launcher") "app-wails"
$PlatformDir = Join-Path $RepoRoot "Platform"

function Test-IsWindowsBinary {
    param(
        [string]$Path
    )

    if (-not (Test-Path $Path)) {
        return $false
    }
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        try {
            if ($stream.Length -lt 2) {
                return $false
            }
            $header = New-Object byte[] 2
            $null = $stream.Read($header, 0, 2)
            return $header[0] -eq 0x4D -and $header[1] -eq 0x5A
        } finally {
            $stream.Dispose()
        }
    } catch {
        return $false
    }
}

function Test-GoCandidate {
    param(
        [string]$Path
    )

    if (-not (Test-IsWindowsBinary $Path)) {
        return $false
    }
    try {
        cmd /c "`"$Path`" version" *> $null
        return $LASTEXITCODE -eq 0
    } catch {
        return $false
    }
}

function Resolve-GoExe {
    $localCandidates = Get-ChildItem -Path (Join-Path $RepoRoot ".tools") -Filter "go.exe" -Recurse -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty FullName
    foreach ($candidate in $localCandidates) {
        if (Test-GoCandidate $candidate) {
            return $candidate
        }
    }

    $cacheRoot = Join-Path $env:USERPROFILE ".cache\codex"
    if (Test-Path $cacheRoot) {
        $cacheCandidates = Get-ChildItem -Path $cacheRoot -Filter "go.exe" -Recurse -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty FullName
        foreach ($candidate in $cacheCandidates) {
            if (Test-GoCandidate $candidate) {
                return $candidate
            }
        }
    }

    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd -and (Test-GoCandidate $cmd.Source)) {
        return $cmd.Source
    }
    throw "Go executable not found. Install Go or place a toolchain under .tools\\go*\\bin\\go.exe"
}

$GoExe = Resolve-GoExe

if (-not $Version) {
    try {
        $Version = & git -C $RepoRoot describe --tags --always --dirty 2>$null
        if (-not $Version) { $Version = "dev" }
    } catch {
        $Version = "dev"
    }
}

$Platform = "Windows-x86_64"
$PackageName = "PinchBot-$Version-$Platform"
$OutDir = Join-Path $DistDir $PackageName
if (Test-Path $OutDir) {
    Remove-Item -Recurse -Force $OutDir
}
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "  PinchBot Release Build" -ForegroundColor Cyan
Write-Host "  Version: $Version  Output: $OutDir" -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan

# 1. Build PinchBot gateway + settings launcher
Write-Host "`n[1/4] Building PinchBot (pinchbot + pinchbot-launcher) ..." -ForegroundColor Yellow
Push-Location $PinchBotDir
try {
    $generateOk = $true
    try {
        & $GoExe generate ./... *> $null
    } catch {
        $generateOk = $false
    }
    if (-not $generateOk -or -not $?) { Write-Host "  go generate warning (ok to ignore)" -ForegroundColor DarkYellow }

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    & $GoExe build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "pinchbot.exe") ./cmd/picoclaw
    if (-not $?) { throw "pinchbot build failed" }

    & $GoExe build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "pinchbot-launcher.exe") ./cmd/picoclaw-launcher
    if (-not $?) { throw "pinchbot-launcher build failed" }
} finally {
    Pop-Location
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
}

# 2. Build Platform backend
Write-Host "`n[2/4] Building Platform backend (platform-server.exe) ..." -ForegroundColor Yellow
if (Test-Path $PlatformDir) {
    Push-Location $PlatformDir
    try {
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        & $GoExe build -ldflags "-s -w" -o (Join-Path $OutDir "platform-server.exe") ./cmd/platform-server
        if (-not $?) { throw "platform-server build failed" }
    } finally {
        Pop-Location
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    }
}

# 3. Build Launcher (Wails tray + chat window)
Write-Host "`n[3/4] Building Launcher (launcher-chat.exe) ..." -ForegroundColor Yellow
Push-Location $LauncherWailsDir
try {
    & $GoExe build -tags "desktop,production" -ldflags "-s -w -H windowsgui -X main.Version=$Version" -o (Join-Path $OutDir "launcher-chat.exe") .
    if (-not $?) { throw "launcher-chat build failed" }
} finally {
    Pop-Location
}

# 4. Copy config examples and write README
Write-Host "`n[4/4] Copying config examples and writing README ..." -ForegroundColor Yellow
$ConfigExampleSrc = Join-Path (Join-Path $PinchBotDir "config") "config.example.json"
$ConfigDir = Join-Path $OutDir "config"
$ConfigExampleDst = Join-Path $ConfigDir "config.example.json"
if (Test-Path $ConfigExampleSrc) {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    Copy-Item -Path $ConfigExampleSrc -Destination $ConfigExampleDst -Force
} else {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
}

$PlatformExampleEnv = Join-Path (Join-Path $PlatformDir "config") "platform.example.env"
if (Test-Path $PlatformExampleEnv) {
    Copy-Item -Path $PlatformExampleEnv -Destination (Join-Path $ConfigDir "platform.example.env") -Force
}
$RuntimeConfigExample = Join-Path (Join-Path $PlatformDir "config") "runtime-config.example.json"
if (Test-Path $RuntimeConfigExample) {
    Copy-Item -Path $RuntimeConfigExample -Destination (Join-Path $ConfigDir "runtime-config.example.json") -Force
}

$ReadmePath = Join-Path $OutDir "README.txt"
$ReadmeContent = @"
PinchBot - Usage
========================================
Version: $Version
Platform: $Platform

FOLDER STRUCTURE (what you are shipping)
----------------------------------------
  launcher-chat.exe       Main program (double-click this)
  pinchbot-launcher.exe   Config UI service (settings starts PinchBot-launcher on demand)
  pinchbot.exe            Gateway (auto-started by launcher-chat.exe)
  platform-server.exe     App account / official-model backend (auto-started after config\platform.env exists)
  config\
    config.example.json   Example config
    platform.example.env  Example platform env (copy to platform.env to enable local backend)
    runtime-config.example.json   Example official-model runtime config
  README.txt              This file

USER DATA (created beside the executables on first run)
-------------------------------------------------------
  .pinchbot\
    config.json           Auto-created default config (workspace defaults to "workspace")
    auth.json             Local provider auth cache
    workspace\            Auto-created workspace with starter files on first gateway start

  You can override the home/config paths with PINCHBOT_HOME / PINCHBOT_CONFIG if needed.

FIRST RUN
---------
  Double-click launcher-chat.exe.
  It creates .pinchbot\ if missing, bootstraps .pinchbot\config.json, and starts:
    - pinchbot.exe gateway
    - platform-server.exe (only when config\platform.env exists)
  The settings page does NOT stay resident by default; settings starts PinchBot-launcher on demand.

MAIN PROGRAM: launcher-chat.exe
  Double-click to run. It auto-starts the gateway and keeps the chat window behind login.
  Tray icon: open chat window; Settings opens http://localhost:18800 and starts pinchbot-launcher.exe on demand.

PLATFORM BACKEND: platform-server.exe
  launcher-chat.exe auto-starts this service from the package root after
  config\platform.env exists.
  The desktop chat window starts behind the auth gate, so launcher-chat itself,
  app account login, official-model billing, and recharge all require it.
  The release package ships example-only templates, so create live config first:
    1) copy config\platform.example.env to config\platform.env
    2) edit PLATFORM_* values for your environment
    3) optionally copy runtime-config.example.json to runtime-config.json as a starting point
       (or let the server create an empty runtime file on first bootstrap)
    4) then launch launcher-chat.exe (or run platform-server.exe manually)

SIGNING
  正式外发前请补充代码签名。
  Windows binaries are not code-signed by this script.
  For external customer distribution, sign the executables and installer/ZIP first.

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

Write-Host "`nPackage '$PackageName' is ready for internal QA. Sign it before external customer distribution.`n" -ForegroundColor Cyan
