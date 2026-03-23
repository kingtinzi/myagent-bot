param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$ConfigDir = Join-Path $RepoRoot "Platform\config"
$ExampleEnv = Join-Path $ConfigDir "platform.example.env"
$LiveEnv = Join-Path $ConfigDir "platform.env"
$ExampleRuntime = Join-Path $ConfigDir "runtime-config.example.json"
$LiveRuntime = Join-Path $ConfigDir "runtime-config.json"

function Copy-IfNeeded {
    param(
        [string]$SourcePath,
        [string]$DestinationPath
    )

    if (-not (Test-Path $SourcePath)) {
        throw "Missing template: $SourcePath"
    }

    if ((Test-Path $DestinationPath) -and (-not $Force)) {
        Write-Host "Skip existing file: $DestinationPath (use -Force to overwrite)" -ForegroundColor Yellow
        return
    }

    Copy-Item -Path $SourcePath -Destination $DestinationPath -Force
    Write-Host "Wrote: $DestinationPath" -ForegroundColor Green
}

New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
Copy-IfNeeded -SourcePath $ExampleEnv -DestinationPath $LiveEnv
Copy-IfNeeded -SourcePath $ExampleRuntime -DestinationPath $LiveRuntime

Write-Host ""
Write-Host "Done." -ForegroundColor Cyan
Write-Host "Templates:" -ForegroundColor Cyan
Write-Host "  - platform.example.env -> platform.env"
Write-Host "  - runtime-config.example.json -> runtime-config.json"
Write-Host "Replace placeholder values, especially replace-with-your-upstream-api-key." -ForegroundColor Yellow
