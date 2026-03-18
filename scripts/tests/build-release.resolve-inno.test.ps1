[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$repoRoot = (Get-Item (Join-Path $PSScriptRoot '..\..')).FullName
$scriptPath = Join-Path $repoRoot 'scripts\build-release.ps1'

if (-not (Test-Path $scriptPath)) {
    throw "build-release script not found: $scriptPath"
}

$parseErrors = $null
$tokens = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors -and $parseErrors.Count -gt 0) {
    throw "failed to parse build-release.ps1: $($parseErrors[0].Message)"
}

$neededFunctions = @(
    'Resolve-InnoSetupCompiler'
)

foreach ($functionName in $neededFunctions) {
    $definition = $ast.Find({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq $functionName
    }, $true)
    if (-not $definition) {
        throw "function $functionName not found in build-release.ps1"
    }
    Invoke-Expression $definition.Extent.Text
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('pinchbot-inno-test-' + [guid]::NewGuid().ToString('N'))
$fakeLocalAppData = Join-Path $tempRoot 'AppData\Local'
$fakeIscc = Join-Path $fakeLocalAppData 'Programs\Inno Setup 6\ISCC.exe'
New-Item -ItemType Directory -Path (Split-Path $fakeIscc -Parent) -Force | Out-Null
Set-Content -Path $fakeIscc -Value 'stub' -Encoding Ascii

$originalLocalAppData = $env:LOCALAPPDATA
$originalProgramFiles = $env:ProgramFiles
$originalProgramFilesX86 = ${env:ProgramFiles(x86)}
$originalPath = $env:Path

try {
    $env:LOCALAPPDATA = $fakeLocalAppData
    $env:ProgramFiles = Join-Path $tempRoot 'Program Files'
    ${env:ProgramFiles(x86)} = Join-Path $tempRoot 'Program Files (x86)'
    $env:Path = ''

    $resolved = Resolve-InnoSetupCompiler
    if ($resolved -ne $fakeIscc) {
        throw "expected Resolve-InnoSetupCompiler to prefer LOCALAPPDATA install path '$fakeIscc', got '$resolved'"
    }

    Write-Host "PASS: Resolve-InnoSetupCompiler found LOCALAPPDATA install path"
} finally {
    if ($null -ne $originalLocalAppData) { $env:LOCALAPPDATA = $originalLocalAppData } else { Remove-Item Env:\LOCALAPPDATA -ErrorAction SilentlyContinue }
    if ($null -ne $originalProgramFiles) { $env:ProgramFiles = $originalProgramFiles } else { Remove-Item Env:\ProgramFiles -ErrorAction SilentlyContinue }
    if ($null -ne $originalProgramFilesX86) { ${env:ProgramFiles(x86)} = $originalProgramFilesX86 } else { Remove-Item 'Env:\ProgramFiles(x86)' -ErrorAction SilentlyContinue }
    if ($null -ne $originalPath) { $env:Path = $originalPath } else { Remove-Item Env:\Path -ErrorAction SilentlyContinue }
    if (Test-Path $tempRoot) {
        Remove-Item -Path $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
