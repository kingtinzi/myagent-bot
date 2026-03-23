# Sign a Windows binary with signtool (Authenticode).
# Prerequisites: Windows SDK signtool.exe, a cert thumbprint, and a RFC3161 timestamp URL.
# Environment:
#   WIN_SIGN_CERT_SHA1     Certificate SHA1 thumbprint (hex)
#   WIN_SIGN_TIMESTAMP_URL Timestamp server, e.g. http://timestamp.digicert.com

param(
    [Parameter(Mandatory = $true)]
    [string]$Path
)

$ErrorActionPreference = "Stop"
$signtool = "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe"
if (-not (Test-Path $signtool)) {
    $signtool = "signtool.exe"
}
$thumb = $env:WIN_SIGN_CERT_SHA1
$ts = $env:WIN_SIGN_TIMESTAMP_URL
if (-not $thumb) {
    throw "WIN_SIGN_CERT_SHA1 is not set"
}
if (-not $ts) {
    throw "WIN_SIGN_TIMESTAMP_URL is not set"
}
if (-not (Test-Path $Path)) {
    throw "File not found: $Path"
}

& $signtool sign /fd sha256 /td sha256 /tr $ts /sha1 $thumb $Path
if ($LASTEXITCODE -ne 0) {
    throw "signtool.exe sign failed with exit $LASTEXITCODE"
}

Get-AuthenticodeSignature -FilePath $Path | Format-List
$status = (Get-AuthenticodeSignature -FilePath $Path).Status
if ($status -ne "Valid") {
    throw "Expected signature status Valid, got: $status"
}
