param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName
$ConfigDir = Join-Path $RepoRoot "Platform\config"

function Copy-TemplateFile {
    param(
        [Parameter(Mandatory = $true)][string]$SourceName,
        [Parameter(Mandatory = $true)][string]$TargetName
    )

    $sourcePath = Join-Path $ConfigDir $SourceName
    $targetPath = Join-Path $ConfigDir $TargetName

    if (-not (Test-Path $sourcePath)) {
        throw "Missing template: $sourcePath"
    }
    if ((Test-Path $targetPath) -and -not $Force) {
        Write-Host "Skip existing $targetPath (use -Force to overwrite)"
        return
    }
    Copy-Item -Path $sourcePath -Destination $targetPath -Force
    Write-Host "Wrote $targetPath"
}

Copy-TemplateFile -SourceName "platform.example.env" -TargetName "platform.env"
Copy-TemplateFile -SourceName "runtime-config.example.json" -TargetName "runtime-config.json"

Write-Host ""
Write-Host "Local platform live config bootstrap complete."
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Edit Platform\config\platform.env and replace live values such as:"
Write-Host "     - PLATFORM_DATABASE_URL"
Write-Host "     - PLATFORM_SUPABASE_URL / PLATFORM_SUPABASE_ANON_KEY"
Write-Host "     - PLATFORM_SUPABASE_JWKS_URL or PLATFORM_SUPABASE_JWT_SECRET"
Write-Host "       (or PLATFORM_SUPABASE_PUBLISHABLE_KEY if you follow Supabase's newer naming)"
Write-Host "     - PLATFORM_EASYPAY_PID / PLATFORM_EASYPAY_KEY (if payment is enabled)"
Write-Host "  2. Edit Platform\config\runtime-config.json and replace placeholders such as:"
Write-Host "     - replace-with-your-upstream-api-key"
Write-Host "     - example agreement URLs"
Write-Host "     - official model pricing/version metadata"
Write-Host "  3. Start the local stack with:"
Write-Host "     - .\scripts\start-local-platform.ps1"
Write-Host ""
Write-Host "Do not ship platform.example.env or runtime-config.example.json as live config."
