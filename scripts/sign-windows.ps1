param(
    [string]$PackageDir = "",
    [string]$InstallerPath = "",
    [string[]]$Files = @(),
    [string]$CertificateThumbprint = $env:WIN_SIGN_CERT_SHA1,
    [string]$TimestampUrl = $(if ($env:WIN_SIGN_TIMESTAMP_URL) { $env:WIN_SIGN_TIMESTAMP_URL } else { "http://timestamp.digicert.com" }),
    [string]$DigestAlgorithm = "sha256"
)

$ErrorActionPreference = "Stop"

function Resolve-SignTool {
    if ($env:WIN_SIGNTOOL_PATH -and (Test-Path $env:WIN_SIGNTOOL_PATH)) {
        return $env:WIN_SIGNTOOL_PATH
    }

    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    $sdkRoots = @(
        "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
        "$env:ProgramFiles\Windows Kits\10\bin"
    ) | Where-Object { $_ -and (Test-Path $_) }

    foreach ($root in $sdkRoots) {
        $candidate = Get-ChildItem -Path $root -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
            Sort-Object FullName -Descending |
            Select-Object -First 1
        if ($candidate) {
            return $candidate.FullName
        }
    }

    throw "signtool.exe not found. Set WIN_SIGNTOOL_PATH or install Windows SDK signing tools."
}

function Get-FilesToSign {
    $items = New-Object System.Collections.Generic.List[string]

    foreach ($file in $Files) {
        if ([string]::IsNullOrWhiteSpace($file)) { continue }
        $resolved = Resolve-Path -LiteralPath $file -ErrorAction Stop
        $items.Add($resolved.Path)
    }

    if (-not [string]::IsNullOrWhiteSpace($PackageDir)) {
        $resolvedPackageDir = (Resolve-Path -LiteralPath $PackageDir -ErrorAction Stop).Path
        Get-ChildItem -Path $resolvedPackageDir -Filter *.exe -File -ErrorAction Stop | ForEach-Object {
            $items.Add($_.FullName)
        }
    }

    if (-not [string]::IsNullOrWhiteSpace($InstallerPath)) {
        $resolvedInstaller = Resolve-Path -LiteralPath $InstallerPath -ErrorAction Stop
        $items.Add($resolvedInstaller.Path)
    }

    $unique = $items |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) } |
        Sort-Object -Unique

    if (-not $unique -or $unique.Count -eq 0) {
        throw "No files selected for signing. Pass -PackageDir, -InstallerPath, or -Files."
    }

    return $unique
}

function Assert-ValidSignature {
    param(
        [string]$Path
    )

    $signature = Get-AuthenticodeSignature -FilePath $Path
    if ($signature.Status -ne "Valid") {
        throw "Authenticode verification failed for '$Path': $($signature.Status)"
    }
}

if ([string]::IsNullOrWhiteSpace($CertificateThumbprint)) {
    throw "Certificate thumbprint is required. Pass -CertificateThumbprint or set WIN_SIGN_CERT_SHA1."
}

$signTool = Resolve-SignTool
$targets = Get-FilesToSign

Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "  PinchBot Windows Signing" -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "signtool.exe: $signTool"
Write-Host "Digest:       $DigestAlgorithm"
Write-Host "Timestamp:    $TimestampUrl"
Write-Host "Targets:"
$targets | ForEach-Object { Write-Host "  - $_" }

foreach ($target in $targets) {
    Write-Host "`nSigning: $target" -ForegroundColor Yellow
    & $signTool sign `
        /sha1 $CertificateThumbprint `
        /fd $DigestAlgorithm `
        /tr $TimestampUrl `
        /td $DigestAlgorithm `
        $target
    if (-not $?) {
        throw "signtool sign failed for $target"
    }

    Write-Host "Verifying: $target" -ForegroundColor Yellow
    & $signTool verify /pa /v $target
    if (-not $?) {
        throw "signtool verify failed for $target"
    }
    Assert-ValidSignature -Path $target
}

Write-Host "`nAll selected files are signed and verified." -ForegroundColor Green
