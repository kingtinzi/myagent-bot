# Generate update-manifest.json for the desktop auto-updater.
# Usage:
#   .\scripts\generate-update-manifest.ps1 -Version "1.0.0" `
#     -ZipPath "dist\PinchBot-1.0.0-Windows-x86_64.zip" `
#     -DownloadURL "https://gitee.com/<owner>/<repo>/releases/download/v1.0.0/PinchBot-1.0.0-Windows-x86_64.zip" `
#     -OutFile "dist\update-manifest.json"

param(
    [Parameter(Mandatory = $true)]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [string]$ZipPath,

    [Parameter(Mandatory = $true)]
    [string]$DownloadURL,

    [string]$ZipFolder = "",
    [string]$ReleaseDate = "",
    [string]$Notes = "",
    [string]$OutFile = ""
)

$ErrorActionPreference = "Stop"

$resolvedZipPath = (Resolve-Path -Path $ZipPath).Path
if (-not (Test-Path $resolvedZipPath)) {
    throw "ZIP not found: $resolvedZipPath"
}

if (-not $ZipFolder.Trim()) {
    $ZipFolder = [System.IO.Path]::GetFileNameWithoutExtension($resolvedZipPath)
}
if (-not $ReleaseDate.Trim()) {
    $ReleaseDate = (Get-Date -Format "yyyy-MM-dd")
}

$sha256 = (Get-FileHash -Path $resolvedZipPath -Algorithm SHA256).Hash.ToLowerInvariant()

$manifest = [ordered]@{
    version      = $Version.Trim()
    url          = $DownloadURL.Trim()
    zip_folder   = $ZipFolder.Trim()
    sha256       = $sha256
    release_date = $ReleaseDate.Trim()
}
if ($Notes.Trim()) {
    $manifest.notes = $Notes.Trim()
}

$json = ($manifest | ConvertTo-Json -Depth 6)

if ($OutFile.Trim()) {
    $outPath = (Resolve-Path -Path (Split-Path -Parent $OutFile) -ErrorAction SilentlyContinue)
    if ($null -ne $outPath) {
        $OutFile = Join-Path $outPath.Path (Split-Path -Leaf $OutFile)
    }
    $json | Set-Content -Path $OutFile -Encoding UTF8
    Write-Host "Wrote manifest: $OutFile" -ForegroundColor Green
} else {
    $json
}

