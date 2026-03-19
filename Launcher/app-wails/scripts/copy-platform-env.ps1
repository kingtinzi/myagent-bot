# Wails postBuildHook on Windows (cwd = build/bin)
$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$AppWails = Split-Path -Parent $ScriptDir
$RepoRoot = Split-Path -Parent (Split-Path -Parent $AppWails)
$Src = Join-Path $RepoRoot "Platform\config\platform.env"
$Dst = Join-Path $AppWails "build\bin\config\platform.env"

if (Test-Path -LiteralPath $Src) {
    $dstDir = Split-Path -Parent $Dst
    if (-not (Test-Path -LiteralPath $dstDir)) {
        New-Item -ItemType Directory -Force -Path $dstDir | Out-Null
    }
    Copy-Item -LiteralPath $Src -Destination $Dst -Force
    Write-Host "[postbuild] Copied Platform/config/platform.env -> build/bin/config/platform.env"
} else {
    Write-Host "[postbuild] Skipped: $Src not found (copy platform.example.env to platform.env)"
}
