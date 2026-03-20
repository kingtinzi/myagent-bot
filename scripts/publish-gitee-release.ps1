[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Tag,

    [string]$GiteeOwner = 'rainboxup',

    [string]$GiteeRepo = 'pinchbot',

    [Parameter(Mandatory = $true)]
    [string]$GiteeToken,

    [Parameter(Mandatory = $true)]
    [string]$WindowsZip,

    [Parameter(Mandatory = $true)]
    [string]$WindowsSetup,

    [Parameter(Mandatory = $true)]
    [string]$MacDmg,

    [string]$Branch = 'main',

    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

$ApiBase
function Mask-AccessTokenInUri {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Uri
    )

    return ($Uri -replace '(access_token=)[^&]+', '${1}***')
}

function Sanitize-AccessTokenInBody {
    param(
        [Parameter(Mandatory = $true)]
        $Body
    )

    if ($Body -is [hashtable]) {
        $copy = @{}
        foreach ($k in $Body.Keys) {
            if ($k -eq 'access_token') {
                $copy[$k] = '***'
            } else {
                $copy[$k] = $Body[$k]
            }
        }
        return $copy
    }

    return $Body
}function Get-ErrorResponseBody {
    param(
        [Parameter(Mandatory = $true)]
        $ErrorRecord
    )

    try {
        $resp = $ErrorRecord.Exception.Response
        if ($null -eq $resp) {
            return ''
        }
        if ($null -ne $resp.Content) {
            return $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        }
        return ''
    } catch {
        return ''
    }
}

function Get-HttpStatusCode {
    param(
        [Parameter(Mandatory = $true)]
        $ErrorRecord
    )

    try {
        $resp = $ErrorRecord.Exception.Response
        if ($null -eq $resp) {
            return $null
        }
        return [int]$resp.StatusCode
    } catch {
        return $null
    }
}

function New-GiteeApiUri {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [hashtable]$Query
    )

    $builder = New-Object System.UriBuilder("$ApiBase/$Path")
    $q = @{}
    if ($null -ne $Query) {
        foreach ($k in $Query.Keys) {
            if ($null -ne $Query[$k] -and [string]$Query[$k] -ne '') {
                $q[$k] = [string]$Query[$k]
            }
        }
    }
    if (-not $q.ContainsKey('access_token')) {
        $q['access_token'] = $GiteeToken
    }

    $pairs = New-Object System.Collections.Generic.List[string]
    foreach ($k in ($q.Keys | Sort-Object)) {
        $pairs.Add("$([uri]::EscapeDataString($k))=$([uri]::EscapeDataString($q[$k]))")
    }
    $builder.Query = ($pairs -join '&')
    return $builder.Uri.AbsoluteUri
}

function Invoke-GiteeJson {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Method,

        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [object]$Body
    )
    if ($DryRun) {
        $safeUri = Mask-AccessTokenInUri $Uri
        Write-Host "[dry-run] $Method $safeUri" -ForegroundColor Yellow
        if ($null -ne $Body) {
            $safeBody = Sanitize-AccessTokenInBody $Body
            Write-Host ("[dry-run] body: " + (($safeBody | ConvertTo-Json -Depth 10))) -ForegroundColor Yellow
        }
        return $null
    }
if ($null -eq $Body) {
        return Invoke-RestMethod -Method $Method -Uri $Uri -TimeoutSec 60
    }

    $json = ($Body | ConvertTo-Json -Depth 20)
    return Invoke-RestMethod -Method $Method -Uri $Uri -ContentType 'application/json' -Body $json -TimeoutSec 60
}

function Invoke-GiteeForm {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Method,

        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [hashtable]$Form
    )
    if ($DryRun) {
        $safeUri = Mask-AccessTokenInUri $Uri
        Write-Host "[dry-run] $Method $safeUri" -ForegroundColor Yellow
        if ($null -ne $Form) {
            Write-Host ("[dry-run] form keys: " + (($Form.Keys | Sort-Object) -join ', ')) -ForegroundColor Yellow
        }
        return $null
    }
return Invoke-RestMethod -Method $Method -Uri $Uri -Form $Form -TimeoutSec 300
}

function Invoke-GiteeJsonWithFallback {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Method,

        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [Parameter(Mandatory = $true)]
        [hashtable]$Body
    )

    try {
        return Invoke-GiteeJson -Method $Method -Uri $Uri -Body $Body
    } catch {
        $status = Get-HttpStatusCode $_
        $body = Get-ErrorResponseBody $_
        Write-Host "[warn] JSON request failed (HTTP $status). Retrying as x-www-form-urlencoded." -ForegroundColor Yellow
        if ($body) {
            Write-Host "[warn] Response: $body" -ForegroundColor Yellow
        }
        return Invoke-RestMethod -Method $Method -Uri $Uri -Body $Body -TimeoutSec 60
    }
}

function Get-ReleaseByTag {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [string]$TagName
    )

    $escapedTag = [uri]::EscapeDataString($TagName)
    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/releases/tags/$escapedTag" -Query @{}
    try {
        return Invoke-RestMethod -Method Get -Uri $uri -TimeoutSec 30
    } catch {
        $status = Get-HttpStatusCode $_
        if ($status -eq 404) {
            return $null
        }
        $body = Get-ErrorResponseBody $_
        throw "Failed to query release by tag (HTTP $status): $body"
    }
}

function Create-Release {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [string]$TagName,
        [Parameter(Mandatory = $true)]
        [string]$TargetCommitish
    )

    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/releases" -Query @{}
    $body = @{
        access_token      = $GiteeToken
        tag_name          = $TagName
        name              = $TagName
        body              = "Automated release from GitHub Actions for $TagName"
        target_commitish  = $TargetCommitish
        prerelease        = $false
    }

    return Invoke-GiteeJsonWithFallback -Method Post -Uri $uri -Body $body
}

function Get-ReleaseAttachFiles {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [int]$ReleaseId
    )

    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/releases/$ReleaseId/attach_files" -Query @{ per_page = 100 }
    try {
        return Invoke-RestMethod -Method Get -Uri $uri -TimeoutSec 30
    } catch {
        $status = Get-HttpStatusCode $_
        $body = Get-ErrorResponseBody $_
        throw "Failed to list attach files (HTTP $status): $body"
    }
}

function Upload-ReleaseAttachment {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [int]$ReleaseId,
        [Parameter(Mandatory = $true)]
        [string]$FilePath
    )

    if (-not (Test-Path -LiteralPath $FilePath)) {
        throw "File not found: $FilePath"
    }

    $fileInfo = Get-Item -LiteralPath $FilePath
    $fileName = $fileInfo.Name

    $existing = @(Get-ReleaseAttachFiles -Owner $Owner -Repo $Repo -ReleaseId $ReleaseId)
    $existingNames = @{}
    foreach ($item in $existing) {
        $n = $item.name
        if (-not $n) { $n = $item.filename }
        if (-not $n) { $n = $item.file_name }
        if ($n) { $existingNames[$n] = $true }
    }

    if ($existingNames.ContainsKey($fileName)) {
        Write-Host "[skip] Attachment already exists: $fileName" -ForegroundColor Cyan
        return
    }

    $uri = "$ApiBase/repos/$Owner/$Repo/releases/$ReleaseId/attach_files"
    $form = @{
        access_token = $GiteeToken
        file         = $fileInfo
    }

    Write-Host "[upload] $fileName" -ForegroundColor Green
    $null = Invoke-GiteeForm -Method Post -Uri $uri -Form $form
}

function Get-RepoFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Ref
    )

    $query = @{ ref = $Ref }
    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/contents/$Path" -Query $query
    try {
        return Invoke-RestMethod -Method Get -Uri $uri -TimeoutSec 30
    } catch {
        $status = Get-HttpStatusCode $_
        if ($status -eq 404) {
            return $null
        }
        $body = Get-ErrorResponseBody $_
        throw "Failed to fetch file contents (HTTP $status): $body"
    }
}

function Put-RepoFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Ref,
        [Parameter(Mandatory = $true)]
        [string]$Sha,
        [Parameter(Mandatory = $true)]
        [string]$Base64Content,
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/contents/$Path" -Query @{}
    $body = @{
        access_token = $GiteeToken
        content      = $Base64Content
        sha          = $Sha
        message      = $Message
        branch       = $Ref
    }

    $null = Invoke-GiteeJsonWithFallback -Method Put -Uri $uri -Body $body
}

function Post-RepoFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Owner,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Ref,
        [Parameter(Mandatory = $true)]
        [string]$Base64Content,
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    $uri = New-GiteeApiUri -Path "repos/$Owner/$Repo/contents/$Path" -Query @{}
    $body = @{
        access_token = $GiteeToken
        content      = $Base64Content
        message      = $Message
        branch       = $Ref
    }

    $null = Invoke-GiteeJsonWithFallback -Method Post -Uri $uri -Body $body
}

$Tag = $Tag.Trim()
if (-not $Tag) {
    throw 'Tag is required.'
}

foreach ($p in @($WindowsZip, $WindowsSetup, $MacDmg)) {
    if (-not (Test-Path -LiteralPath $p)) {
        throw "File not found: $p"
    }
}

Write-Host "==> Gitee publish" -ForegroundColor Green
Write-Host "    Repo: $GiteeOwner/$GiteeRepo" -ForegroundColor Green
Write-Host "    Tag:  $Tag" -ForegroundColor Green

$release = Get-ReleaseByTag -Owner $GiteeOwner -Repo $GiteeRepo -TagName $Tag
if (-null -eq $release) {
    Write-Host "==> Release not found for tag '$Tag', creating ..." -ForegroundColor Yellow
    $release = Create-Release -Owner $GiteeOwner -Repo $GiteeRepo -TagName $Tag -TargetCommitish $Branch
}

if (-null -eq $release -or -not $release.id) {
    throw 'Failed to resolve release id.'
}

$releaseId = [int]$release.id
Write-Host "==> Release id: $releaseId" -ForegroundColor Green

Upload-ReleaseAttachment -Owner $GiteeOwner -Repo $GiteeRepo -ReleaseId $releaseId -FilePath $WindowsSetup
Upload-ReleaseAttachment -Owner $GiteeOwner -Repo $GiteeRepo -ReleaseId $releaseId -FilePath $WindowsZip
Upload-ReleaseAttachment -Owner $GiteeOwner -Repo $GiteeRepo -ReleaseId $releaseId -FilePath $MacDmg

$zipName = (Get-Item -LiteralPath $WindowsZip).Name
$zipFolder = [System.IO.Path]::GetFileNameWithoutExtension($zipName)
$sha256 = (Get-FileHash -LiteralPath $WindowsZip -Algorithm SHA256).Hash.ToLowerInvariant()
$downloadUrl = "https://gitee.com/$GiteeOwner/$GiteeRepo/releases/download/$Tag/$zipName"

$manifest = [ordered]@{
    version      = $Tag
    url          = $downloadUrl
    zip_folder   = $zipFolder
    sha256       = $sha256
    release_date = (Get-Date -Format 'yyyy-MM-dd')
}

$manifestJson = ($manifest | ConvertTo-Json -Depth 6)
$manifestJsonWithNewline = $manifestJson + "`n"
$base64 = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($manifestJsonWithNewline))

$manifestPath = 'update/update-manifest.json'
$commitMsg = "chore(update): bump update manifest to $Tag"

Write-Host "==> Updating $manifestPath on branch '$Branch' ..." -ForegroundColor Yellow

$current = Get-RepoFile -Owner $GiteeOwner -Repo $GiteeRepo -Path $manifestPath -Ref $Branch
if ($null -eq $current) {
    Write-Host "==> Manifest file not found on Gitee; creating ..." -ForegroundColor Yellow
    Post-RepoFile -Owner $GiteeOwner -Repo $GiteeRepo -Path $manifestPath -Ref $Branch -Base64Content $base64 -Message $commitMsg
} else {
    if (-not $current.sha) {
        throw 'Gitee contents API returned no sha for update-manifest.'
    }
    Put-RepoFile -Owner $GiteeOwner -Repo $GiteeRepo -Path $manifestPath -Ref $Branch -Sha $current.sha -Base64Content $base64 -Message $commitMsg
}

Write-Host "==> Done." -ForegroundColor Green
Write-Host "    Release: https://gitee.com/$GiteeOwner/$GiteeRepo/releases/tag/$Tag" -ForegroundColor Green
Write-Host "    Manifest url: https://gitee.com/$GiteeOwner/$GiteeRepo/raw/$Branch/$manifestPath" -ForegroundColor Green


