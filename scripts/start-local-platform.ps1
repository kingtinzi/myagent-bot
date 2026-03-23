param(
    [string]$PlatformEnv = "",
    [string]$RuntimeConfig = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$PlatformDir = Join-Path $RepoRoot "Platform"
$ConfigDir = Join-Path $PlatformDir "config"

if (-not $PlatformEnv) {
    $PlatformEnv = Join-Path $ConfigDir "platform.env"
}
if ($PlatformEnv -like "*platform.example.env*") {
    throw "platform.example.env is example-only. Specify -PlatformEnv explicitly."
}
if (-not (Test-Path $PlatformEnv)) {
    throw "Missing env file: $PlatformEnv. Run .\scripts\bootstrap-local-platform-config.ps1 first."
}

if (-not $RuntimeConfig) {
    $RuntimeConfig = Join-Path $ConfigDir "runtime-config.json"
}

if (-not $env:PINCHBOT_HOME) {
    $env:PINCHBOT_HOME = Join-Path $RepoRoot ".openclaw"
}
if (-not $env:PINCHBOT_CONFIG) {
    $env:PINCHBOT_CONFIG = Join-Path $env:PINCHBOT_HOME "config.json"
}
if (-not $env:PLATFORM_RUNTIME_CONFIG_PATH) {
    $env:PLATFORM_RUNTIME_CONFIG_PATH = $RuntimeConfig
}

Get-Content -Path $PlatformEnv | ForEach-Object {
    $line = $_.Trim()
    if (-not $line -or $line.StartsWith("#")) {
        return
    }
    $parts = $line -split "=", 2
    if ($parts.Count -eq 2) {
        $key = $parts[0].Trim()
        $value = $parts[1]
        Set-Item -Path ("Env:\" + $key) -Value $value
    }
}

Write-Host "PINCHBOT_HOME=$($env:PINCHBOT_HOME)"
Write-Host "PINCHBOT_CONFIG=$($env:PINCHBOT_CONFIG)"
Write-Host "PLATFORM_RUNTIME_CONFIG_PATH=$($env:PLATFORM_RUNTIME_CONFIG_PATH)"

Push-Location $PlatformDir
try {
    go run ./cmd/platform-server
} finally {
    Pop-Location
}
