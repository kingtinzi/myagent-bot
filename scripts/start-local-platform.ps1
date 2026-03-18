param(
    [string]$PlatformEnv = ""
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

if (-not $PlatformEnv) {
    $liveEnv = Join-Path $RepoRoot "Platform\\config\\platform.env"
    if (Test-Path $liveEnv) {
        $PlatformEnv = $liveEnv
    } else {
        throw "Missing live Platform config at $liveEnv. Copy Platform\\config\\platform.example.env to platform.env or Specify -PlatformEnv explicitly."
    }
}

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

if ([System.IO.Path]::IsPathRooted($PlatformEnv)) {
    Import-EnvFile $PlatformEnv
} else {
    Import-EnvFile (Join-Path $RepoRoot $PlatformEnv)
}

if (-not $env:PINCHBOT_HOME) {
    $env:PINCHBOT_HOME = Join-Path $RepoRoot ".pinchbot"
} elseif (-not [System.IO.Path]::IsPathRooted($env:PINCHBOT_HOME)) {
    $env:PINCHBOT_HOME = Join-Path $RepoRoot $env:PINCHBOT_HOME
}
if (-not $env:PINCHBOT_CONFIG) {
    $env:PINCHBOT_CONFIG = Join-Path $env:PINCHBOT_HOME "config.json"
} elseif (-not [System.IO.Path]::IsPathRooted($env:PINCHBOT_CONFIG)) {
    $env:PINCHBOT_CONFIG = Join-Path $RepoRoot $env:PINCHBOT_CONFIG
}
if (-not (Test-Path $env:PINCHBOT_HOME)) {
    New-Item -ItemType Directory -Path $env:PINCHBOT_HOME -Force | Out-Null
}

Write-Host "Starting platform-server and launcher-chat (settings will be hosted inside launcher-chat on demand)..."

$platform = Start-Process -FilePath $GoExe -ArgumentList @("run","./cmd/platform-server") -PassThru -WorkingDirectory (Join-Path $RepoRoot "Platform")
$chat = Start-Process -FilePath $GoExe -ArgumentList @("run","-tags","desktop,production",".") -PassThru -WorkingDirectory (Join-Path $RepoRoot "Launcher\app-wails")

Write-Host "platform-server PID=$($platform.Id)"
Write-Host "launcher-chat PID=$($chat.Id)"
Write-Host "Use Stop-Process on the PIDs above when done."
