param(
    [Parameter(Mandatory = $true)]
    [string]$PackageDir,
    [string]$InstallerPath = "",
    [string[]]$ExtraFiles = @()
)

$ErrorActionPreference = "Stop"

$CertSha1 = $env:WIN_SIGN_CERT_SHA1
$TimestampUrl = $env:WIN_SIGN_TIMESTAMP_URL
if (-not $CertSha1) {
    throw "WIN_SIGN_CERT_SHA1 is required."
}
if (-not $TimestampUrl) {
    throw "WIN_SIGN_TIMESTAMP_URL is required."
}

$SignTool = if ($env:WIN_SIGNTOOL_PATH) { $env:WIN_SIGNTOOL_PATH } else { "signtool.exe" }
if (-not (Test-Path $PackageDir)) {
    throw "PackageDir does not exist: $PackageDir"
}

$targets = New-Object System.Collections.Generic.List[string]
foreach ($name in @("launcher-chat.exe", "pinchbot.exe", "pinchbot-launcher.exe", "platform-server.exe")) {
    $path = Join-Path $PackageDir $name
    if (Test-Path $path) {
        $targets.Add((Resolve-Path $path).Path)
    }
}
if ($InstallerPath) {
    if (-not (Test-Path -LiteralPath $InstallerPath)) {
        throw "InstallerPath does not exist: $InstallerPath"
    }
    $targets.Add((Resolve-Path -LiteralPath $InstallerPath).Path)
}
foreach ($extra in $ExtraFiles) {
    if (Test-Path $extra) {
        $targets.Add((Resolve-Path $extra).Path)
    } else {
        throw "Extra file not found: $extra"
    }
}

if ($targets.Count -eq 0) {
    throw "No files to sign."
}

foreach ($target in $targets) {
    Write-Host "Signing: $target" -ForegroundColor Cyan
    & $SignTool sign /sha1 $CertSha1 /fd sha256 /td sha256 /tr $TimestampUrl /v $target
    if ($LASTEXITCODE -ne 0) {
        throw "signtool.exe sign failed for $target (exit $LASTEXITCODE)"
    }

    & $SignTool verify /pa /v $target
    if ($LASTEXITCODE -ne 0) {
        throw "signtool.exe verify failed for $target (exit $LASTEXITCODE)"
    }

    $signature = Get-AuthenticodeSignature -FilePath $target
    if ($signature.Status -ne "Valid") {
        throw "Get-AuthenticodeSignature status is not Valid for ${target}: $($signature.Status)"
    }
}

Write-Host "All signatures are Valid." -ForegroundColor Green
