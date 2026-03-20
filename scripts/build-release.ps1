# PinchBot release build - produces a folder, optional ZIP, and optional per-user Windows installer.
# Usage: .\scripts\build-release.ps1 [-Version "1.0.0"] [-Zip] [-Installer] [-IncludeLivePlatformConfig] [-PlatformAPIBaseURL "https://platform.example.com"] [-BuildPlatformServer] [-NpmRegistry "https://registry.npmmirror.com"]
# Output: dist\PinchBot-<version>-Windows-x86_64\ (exe files + README)

param(
    [string]$Version = "",
    [switch]$Zip,
    [switch]$Installer,
    [switch]$IncludeLivePlatformConfig,
    [string]$PlatformAPIBaseURL = "",
    [switch]$BuildPlatformServer,
    [string]$NpmRegistry = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$DistDir = Join-Path $RepoRoot "dist"
$PinchBotDir = Join-Path $RepoRoot "PinchBot"
$LauncherWailsDir = Join-Path (Join-Path $RepoRoot "Launcher") "app-wails"
$PlatformDir = Join-Path $RepoRoot "Platform"
$PlatformConfigDir = Join-Path $PlatformDir "config"
$PlatformExampleEnv = Join-Path $PlatformConfigDir "platform.example.env"
$PlatformLiveEnv = Join-Path $PlatformConfigDir "platform.env"
$RuntimeConfigExample = Join-Path $PlatformConfigDir "runtime-config.example.json"
$RuntimeConfigLive = Join-Path $PlatformConfigDir "runtime-config.json"

function Get-DotEnvValue {
    param(
        [string]$Path,
        [string]$Key
    )

    if (-not (Test-Path $Path)) {
        return ""
    }
    $needle = "$Key="
    foreach ($line in Get-Content -Path $Path) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith("#")) {
            continue
        }
        if ($trimmed.StartsWith($needle)) {
            return $trimmed.Substring($needle.Length).Trim()
        }
    }
    return ""
}

# Windows PowerShell 5.1 treats native stderr as terminating when ErrorActionPreference is Stop; go/npm are chatty on stderr.
function Invoke-NativeCommand {
    param(
        [ScriptBlock]$Command
    )
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        # Swallow success-stream output so the function's return value is only the exit code (npm.ps1 prints to stdout).
        $null = & $Command
        if ($null -eq $LASTEXITCODE) { return 0 }
        return [int]$LASTEXITCODE
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Test-NpmRegistry {
    param(
        [string]$RegistryURL
    )
    if (-not $RegistryURL) {
        return $false
    }
    $code = Invoke-NativeCommand { cmd /c "npm ping --registry=$RegistryURL" }
    return $code -eq 0
}

function Resolve-NpmRegistry {
    param(
        [string]$Preferred
    )
    $candidates = New-Object System.Collections.Generic.List[string]
    if ($Preferred) {
        $candidates.Add($Preferred.Trim())
    }
    try {
        $current = (cmd /c "npm config get registry" | Select-Object -Last 1).Trim()
        if ($current) {
            $candidates.Add($current)
        }
    } catch {}
    $candidates.Add("https://registry.npmmirror.com")
    $candidates.Add("https://registry.npmjs.org")

    foreach ($url in $candidates) {
        if (Test-NpmRegistry -RegistryURL $url) {
            return $url
        }
    }
    return ""
}

# Copy a Node extension without node_modules (avoids Windows MAX_PATH failures on Copy-Item), then install prod deps in the bundle.
function Sync-BundledNodeExtension {
    param(
        [string]$ExtName,
        [string]$ExtensionsRoot,
        [string]$OutDirRoot,
        [string]$RegistryURL = ""
    )
    $src = Join-Path $ExtensionsRoot $ExtName
    $dst = Join-Path (Join-Path $OutDirRoot "extensions") $ExtName
    if (-not (Test-Path $src)) {
        Write-Warning "Extension not found at $src (skip $ExtName)"
        return
    }
    Write-Host "  Bundling extensions/$ExtName (robocopy, exclude node_modules + npm omit dev) ..." -ForegroundColor DarkCyan
    if (Test-Path $dst) {
        Remove-Item $dst -Recurse -Force -ErrorAction SilentlyContinue
    }
    $extParent = Split-Path $dst
    New-Item -ItemType Directory -Path $extParent -Force | Out-Null
    $null = cmd /c "robocopy `"$src`" `"$dst`" /E /XD node_modules /NFL /NDL /NJH /NJS /NP"
    $rc = $LASTEXITCODE
    if ($null -eq $rc) { $rc = 0 }
    if ($rc -ge 8) {
        throw "robocopy failed for extensions/$ExtName (exit $rc)"
    }
    Push-Location $dst
    try {
        $registryArg = ""
        if ($RegistryURL) {
            $registryArg = " --registry=$RegistryURL"
        }
        $npmExit = Invoke-NativeCommand { cmd /c "npm ci --omit=dev$registryArg" }
        if ($npmExit -ne 0) {
            Write-Warning "npm ci --omit=dev failed in extensions/$ExtName (exit $npmExit), falling back to npm install --omit=dev"
            $npmInstall = Invoke-NativeCommand { cmd /c "npm install --omit=dev$registryArg" }
            if ($npmInstall -ne 0) { throw "npm install --omit=dev failed in extensions/$ExtName (exit $npmInstall)" }
        }
    } finally {
        Pop-Location
    }
}

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

function Resolve-InnoSetupCompiler {
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe"),
        (Join-Path $env:USERPROFILE "AppData\Local\Programs\Inno Setup 6\ISCC.exe"),
        (Join-Path $env:ProgramFiles "Inno Setup 6\ISCC.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "Inno Setup 6\ISCC.exe")
    ) | Where-Object { $_ -and (Test-Path $_) }

    foreach ($candidate in $candidates) {
        return $candidate
    }

    $cmd = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }
    return $null
}

function Get-InstallerAppVersion {
    param(
        [string]$RawVersion
    )

    if ($null -eq $RawVersion) {
        $RawVersion = ""
    }
    $matches = [regex]::Matches($RawVersion, '\d+')
    $parts = @()
    foreach ($match in $matches) {
        if ($parts.Count -ge 4) { break }
        $parts += [string]([int]$match.Value)
    }
    while ($parts.Count -lt 4) {
        $parts += "0"
    }
    return ($parts -join ".")
}

function Get-InstallerOutputVersion {
    param(
        [string]$RawVersion
    )

    if ($null -eq $RawVersion) {
        $RawVersion = ""
    }
    $safe = [regex]::Replace($RawVersion.Trim(), '[^0-9A-Za-z._-]+', '-')
    $safe = $safe.Trim('-')
    if (-not $safe) {
        return "dev"
    }
    return $safe
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
    # Use cmd rmdir first to avoid long-path deletion failures in deep node_modules trees.
    cmd /c "rmdir /s /q `"$OutDir`"" *> $null
    if (Test-Path $OutDir) {
        Remove-Item -Recurse -Force $OutDir
    }
}
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "  PinchBot Release Build" -ForegroundColor Cyan
Write-Host "  Version: $Version  Output: $OutDir" -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan

$PinnedPlatformAPIBaseURL = $PlatformAPIBaseURL.Trim()
if (-not $PinnedPlatformAPIBaseURL -and $IncludeLivePlatformConfig) {
    $PinnedPlatformAPIBaseURL = (Get-DotEnvValue -Path $PlatformLiveEnv -Key "PLATFORM_PUBLIC_BASE_URL").Trim()
    if (-not $PinnedPlatformAPIBaseURL) {
        $platformAddr = (Get-DotEnvValue -Path $PlatformLiveEnv -Key "PLATFORM_ADDR").Trim()
        if ($platformAddr) {
            $PinnedPlatformAPIBaseURL = "http://$platformAddr"
        }
    }
}

if ($PinnedPlatformAPIBaseURL) {
    Write-Host "  Pinned Platform API: $PinnedPlatformAPIBaseURL" -ForegroundColor DarkCyan
} else {
    Write-Host "  Pinned Platform API: <none, runtime auto-resolve>" -ForegroundColor DarkYellow
}
$EffectiveNpmRegistry = Resolve-NpmRegistry -Preferred $NpmRegistry
if ($EffectiveNpmRegistry) {
    Write-Host "  NPM registry: $EffectiveNpmRegistry" -ForegroundColor DarkCyan
} else {
    Write-Warning "No reachable npm registry detected; npm ci may fail."
}

# 1. Build PinchBot gateway + optional standalone settings launcher
Write-Host "`n[1/4] Building PinchBot (pinchbot + optional pinchbot-launcher) ..." -ForegroundColor Yellow
$PluginHostAssets = Join-Path $PinchBotDir "pkg\plugins\assets"
if (Get-Command npm -ErrorAction SilentlyContinue) {
    Write-Host "  (npm ci: pkg/plugins/assets — Node plugin host)" -ForegroundColor DarkCyan
    Push-Location $PluginHostAssets
    try {
        $registryArg = ""
        if ($EffectiveNpmRegistry) {
            $registryArg = " --registry=$EffectiveNpmRegistry"
        }
        # Use cmd so %ERRORLEVEL% is reliable (npm.ps1 can leave $LASTEXITCODE wrong).
        $npmCode = Invoke-NativeCommand { cmd /c "npm ci$registryArg" }
        if ($npmCode -ne 0) { throw "npm ci failed in pkg/plugins/assets (exit $npmCode)" }
    } finally {
        Pop-Location
    }
} else {
    Write-Warning "npm not found; plugin-host will miss node_modules (plugins.node_host)."
}
Push-Location $PinchBotDir
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    $c1 = Invoke-NativeCommand { & $GoExe build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "pinchbot.exe") ./cmd/picoclaw }
    if ($c1 -ne 0) { throw "pinchbot build failed" }

    $c2 = Invoke-NativeCommand { & $GoExe build -tags stdjson -ldflags "-s -w" -o (Join-Path $OutDir "pinchbot-launcher.exe") ./cmd/picoclaw-launcher }
    if ($c2 -ne 0) { throw "pinchbot-launcher build failed" }
} finally {
    Pop-Location
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
}
$PluginHostDst = Join-Path $OutDir "plugin-host"
if (Test-Path $PluginHostDst) {
    Remove-Item $PluginHostDst -Recurse -Force
}
Write-Host "  Copying plugin-host -> $PluginHostDst" -ForegroundColor DarkCyan
Copy-Item -Path $PluginHostAssets -Destination $PluginHostDst -Recurse

$ExtRoot = Join-Path $PinchBotDir "extensions"
Sync-BundledNodeExtension -ExtName "lobster" -ExtensionsRoot $ExtRoot -OutDirRoot $OutDir -RegistryURL $EffectiveNpmRegistry

# 2. Build Platform backend (optional; default off for remote platform deployments)
if ($BuildPlatformServer) {
    Write-Host "`n[2/4] Building Platform backend (platform-server.exe) ..." -ForegroundColor Yellow
    if (Test-Path $PlatformDir) {
        Push-Location $PlatformDir
        try {
            $env:CGO_ENABLED = "0"
            $env:GOOS = "windows"
            $env:GOARCH = "amd64"
            $c3 = Invoke-NativeCommand { & $GoExe build -ldflags "-s -w" -o (Join-Path $OutDir "platform-server.exe") ./cmd/platform-server }
            if ($c3 -ne 0) { throw "platform-server build failed" }
        } finally {
            Pop-Location
            Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
            Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
        }
    }
} else {
    Write-Host "`n[2/4] Skipping platform-server.exe build (remote platform mode)." -ForegroundColor Yellow
}

# 3. Build Launcher (Wails tray + chat window)
Write-Host "`n[3/4] Building Launcher (launcher-chat.exe) ..." -ForegroundColor Yellow
Push-Location $LauncherWailsDir
try {
    $launcherLdflags = "-s -w -H windowsgui -X main.Version=$Version"
    if ($PinnedPlatformAPIBaseURL) {
        $launcherLdflags += " -X main.PlatformAPIBaseURL=$PinnedPlatformAPIBaseURL"
    }
    $c4 = Invoke-NativeCommand { & $GoExe build -tags "desktop,production" -ldflags $launcherLdflags -o (Join-Path $OutDir "launcher-chat.exe") . }
    if ($c4 -ne 0) { throw "launcher-chat build failed" }
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
$GraphMemoryExampleSrc = Join-Path (Join-Path $PinchBotDir "config") "config.graph-memory.example.json"
if (Test-Path $GraphMemoryExampleSrc) {
    Copy-Item -Path $GraphMemoryExampleSrc -Destination (Join-Path $ConfigDir "config.graph-memory.example.json") -Force
}

if (Test-Path $PlatformExampleEnv) {
    Copy-Item -Path $PlatformExampleEnv -Destination (Join-Path $ConfigDir "platform.example.env") -Force
}
if ($IncludeLivePlatformConfig) {
    if (Test-Path $PlatformLiveEnv) {
        Copy-Item -Path $PlatformLiveEnv -Destination (Join-Path $ConfigDir "platform.env") -Force
    } else {
        Write-Warning "IncludeLivePlatformConfig was specified, but no live env exists at $PlatformLiveEnv"
    }
}
if (Test-Path $RuntimeConfigExample) {
    Copy-Item -Path $RuntimeConfigExample -Destination (Join-Path $ConfigDir "runtime-config.example.json") -Force
}
if ($IncludeLivePlatformConfig) {
    if (Test-Path $RuntimeConfigLive) {
        Copy-Item -Path $RuntimeConfigLive -Destination (Join-Path $ConfigDir "runtime-config.json") -Force
    } elseif (-not (Test-Path $RuntimeConfigExample)) {
        Write-Warning "IncludeLivePlatformConfig was specified, but no live runtime config exists at $RuntimeConfigLive"
    }
}

$LauncherPlatformBindingLine = if ($PinnedPlatformAPIBaseURL) {
    "  launcher-chat.exe is pinned to Platform API: $PinnedPlatformAPIBaseURL"
} else {
    "  launcher-chat.exe resolves Platform API at runtime (env/config/default fallback)."
}

$PlatformBinaryLine = if ($BuildPlatformServer) {
    "  platform-server.exe     App account / official-model backend (auto-started after config\platform.env exists)"
} else {
    "  platform-server.exe     Not bundled by default (use remote platform API). Build with -BuildPlatformServer if needed."
}

$PlatformFirstRunLine = if ($BuildPlatformServer) {
    "    - platform-server.exe (only when config\platform.env exists)"
} else {
    "    - no local platform-server.exe in this package (configure remote API via config\platform.env)"
}

$PlatformSection = if ($BuildPlatformServer) {
@"
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
  Internal QA tip:
    If Platform\config\platform.env already exists on the build machine, you can run
      .\scripts\build-release.ps1 -Version 1.0.0 -Zip -IncludeLivePlatformConfig -BuildPlatformServer
    to bundle the live platform config into dist\config\platform.env for local QA only.
"@
} else {
@"
PLATFORM BACKEND: remote mode (default)
  This package does NOT include platform-server.exe by default.
  Configure a remote platform API address in config\platform.env, for example:
    PICOCLAW_PLATFORM_API_BASE_URL=https://platform.example.com
  launcher-chat.exe resolves this URL with fallback priority:
    ldflags pin (-PlatformAPIBaseURL) -> env PICOCLAW_PLATFORM_API_BASE_URL -> config.json platform_api.base_url -> built-in default.
  If you need local backend packaging for internal QA, rebuild with:
      .\scripts\build-release.ps1 -Version 1.0.0 -Zip -BuildPlatformServer -IncludeLivePlatformConfig
"@
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
  pinchbot-launcher.exe   Optional standalone settings service (manual/debug use)
  pinchbot.exe            Optional standalone gateway binary (manual/debug use)
$PlatformBinaryLine
  extensions\lobster      Node lobster workflow plugin (npm prod deps)
  config\
    config.example.json   Example config
    config.graph-memory.example.json   Example graph-memory sidecar (copy to .openclaw\config.graph-memory.json to enable)
    platform.example.env  Example platform env (copy to platform.env to enable local backend)
    platform.env          Optional live platform env when -IncludeLivePlatformConfig is used for internal QA builds
    runtime-config.example.json   Example official-model runtime config
    runtime-config.json   Optional live runtime config when -IncludeLivePlatformConfig is used
  README.txt              This file

USER DATA (created beside the executables on first run)
-------------------------------------------------------
  .openclaw\
    config.json           Auto-created default config (workspace defaults to "workspace")
    auth.json             Local provider auth cache
    workspace\            Auto-created workspace with starter files on first gateway start

  You can override the home/config paths with PINCHBOT_HOME / PINCHBOT_CONFIG if needed.

  Graph-memory: live file is .openclaw\config.graph-memory.json next to config.json, or set PINCHBOT_GRAPH_MEMORY_CONFIG to an absolute path. See config\config.graph-memory.example.json in this package.

FIRST RUN
---------
  Double-click launcher-chat.exe.
  It creates .openclaw\ if missing, bootstraps .openclaw\config.json, and starts:
    - embedded gateway inside launcher-chat.exe
$PlatformFirstRunLine
  The settings page is hosted inside launcher-chat.exe on demand (port 18800).
  pinchbot-launcher.exe remains available only for standalone debugging / compatibility.

MAIN PROGRAM: launcher-chat.exe
  Double-click to run. It hosts the local chat gateway in-process and keeps the chat window behind login.
  Tray icon: open chat window; Settings opens http://localhost:18800 served by launcher-chat.exe itself.
$LauncherPlatformBindingLine

$PlatformSection

SIGNING
  正式外发前请补充代码签名。
  Windows binaries are not code-signed by this script.
  For external customer distribution, sign the executables and installer/ZIP first.

WINDOWS INSTALLER
  Optional: run this script with -Installer to build a per-user Inno Setup installer.
  The installer defaults to %LOCALAPPDATA%\Programs\PinchBot and allows changing
  the installation directory during setup. For the smoothest .openclaw executable-
  local data experience, prefer a user-writable directory instead of Program Files.
  Example:
    .\scripts\build-release.ps1 -Version 1.0.0 -Zip -Installer

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

if ($Installer) {
    $InstallerScript = Join-Path $RepoRoot "scripts\windows-installer.iss"
    if (-not (Test-Path $InstallerScript)) {
        throw "Installer script not found: $InstallerScript"
    }
    $IsccExe = Resolve-InnoSetupCompiler
    if (-not $IsccExe) {
        throw "Inno Setup compiler (ISCC.exe) not found. Install Inno Setup 6 under LOCALAPPDATA/Program Files, or add ISCC.exe to PATH."
    }
    $InstallerAppVersion = Get-InstallerAppVersion $Version
    $InstallerOutputVersion = Get-InstallerOutputVersion $Version
    Write-Host "`nCreating Windows installer via Inno Setup ..." -ForegroundColor Yellow
    $c5 = Invoke-NativeCommand {
        & $IsccExe `
            "/DMyAppVersion=$InstallerAppVersion" `
            "/DMyOutputVersion=$InstallerOutputVersion" `
            "/DMyPackageDir=$OutDir" `
            "/DMyOutputDir=$DistDir" `
            $InstallerScript
    }
    if ($c5 -ne 0) { throw "installer build failed" }
}

Write-Host "`nPackage '$PackageName' is ready for internal QA. Sign it before external customer distribution.`n" -ForegroundColor Cyan
