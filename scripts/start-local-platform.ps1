# Start Platform server from a dev checkout with predictable local state.
# Sets PINCHBOT_HOME and PINCHBOT_CONFIG so `go run` uses this repo (not your global home).
# Usage (from repo root): .\scripts\start-local-platform.ps1

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$env:PINCHBOT_HOME = Join-Path $RepoRoot ".openclaw-dev"
$env:PINCHBOT_CONFIG = Join-Path $RepoRoot "PinchBot\config.json"
New-Item -ItemType Directory -Force -Path $env:PINCHBOT_HOME | Out-Null
Write-Host "PINCHBOT_HOME=$($env:PINCHBOT_HOME)"
Write-Host "PINCHBOT_CONFIG=$($env:PINCHBOT_CONFIG)"
Set-Location (Join-Path $RepoRoot "Platform")
go run ./cmd/platform-server
