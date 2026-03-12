param(
    [string]$PlatformEnv = ".\Platform\config\platform.example.env"
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Get-Item $PSScriptRoot).Parent.FullName

function Resolve-GoExe {
    $localGo = Get-ChildItem -Path (Join-Path $RepoRoot ".tools") -Filter "go.exe" -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty FullName
    if ($localGo) {
        return $localGo
    }
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }
    throw "Go executable not found. Install Go or place a toolchain under .tools\\go*\\bin\\go.exe"
}

$GoExe = Resolve-GoExe

function Import-EnvFile([string]$Path) {
    if (-not (Test-Path $Path)) { return }
    Get-Content $Path | ForEach-Object {
        $line = $_.Trim()
        if (-not $line -or $line.StartsWith("#")) { return }
        $parts = $line.Split("=", 2)
        if ($parts.Count -eq 2) {
            [Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1].Trim(), "Process")
        }
    }
}

Import-EnvFile (Join-Path $RepoRoot $PlatformEnv)

Write-Host "Starting platform-server, picoclaw-launcher, and launcher-chat..."

$platform = Start-Process -FilePath $GoExe -ArgumentList @("run","./cmd/platform-server") -PassThru -WorkingDirectory (Join-Path $RepoRoot "Platform")
$launcher = Start-Process -FilePath $GoExe -ArgumentList @("run","./cmd/picoclaw-launcher") -PassThru -WorkingDirectory (Join-Path $RepoRoot "PicoClaw")
$chat = Start-Process -FilePath $GoExe -ArgumentList @("run","-tags","desktop,production",".") -PassThru -WorkingDirectory (Join-Path $RepoRoot "Launcher\app-wails")

Write-Host "platform-server PID=$($platform.Id)"
Write-Host "picoclaw-launcher PID=$($launcher.Id)"
Write-Host "launcher-chat PID=$($chat.Id)"
Write-Host "Use Stop-Process on the PIDs above when done."
