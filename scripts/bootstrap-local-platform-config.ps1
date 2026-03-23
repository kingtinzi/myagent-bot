# Copy Platform example env/runtime templates into live files for local development.
# Usage: .\scripts\bootstrap-local-platform-config.ps1 [-Force]

param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$Cfg = Join-Path $RepoRoot "Platform\config"
$exEnv = Join-Path $Cfg "platform.example.env"
$liveEnv = Join-Path $Cfg "platform.env"
$exRt = Join-Path $Cfg "runtime-config.example.json"
$liveRt = Join-Path $Cfg "runtime-config.json"

if (-not (Test-Path $exEnv)) {
    throw "Missing template: $exEnv"
}
if (-not (Test-Path $exRt)) {
    throw "Missing template: $exRt"
}
if ((Test-Path $liveEnv) -and -not $Force) {
    throw "platform.env already exists. Re-run with -Force to overwrite."
}
if ((Test-Path $liveRt) -and -not $Force) {
    throw "runtime-config.json already exists. Re-run with -Force to overwrite."
}

Copy-Item -Path $exEnv -Destination $liveEnv -Force:$Force
Copy-Item -Path $exRt -Destination $liveRt -Force:$Force
Write-Host "Created platform.env and runtime-config.json from platform.example.env and runtime-config.example.json"
Write-Host "Next: replace-with-your-upstream-api-key and other PLATFORM_* placeholders in both files."
