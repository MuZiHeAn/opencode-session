<#
.SYNOPSIS
  Publish this repository to GitHub and cut the first release tag.

.DESCRIPTION
  Runs local checks, aligns the Go module path with the target repository,
  commits the working tree, pushes to GitHub, and optionally pushes a
  version tag that triggers the release workflow.

.EXAMPLE
  .\scripts\publish.ps1 -RepoUrl https://github.com/yourname/opencode-session.git

.EXAMPLE
  .\scripts\publish.ps1 -RepoUrl https://github.com/yourname/opencode-session.git -Version v0.1.0 -SkipTag
#>
param(
    [Parameter(Mandatory = $true)]
    [string]$RepoUrl,

    [string]$Version = "v0.1.0",

    [string]$CommitMessage = "Initial release",

    [switch]$SkipVerify,

    [switch]$SkipTag
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

function Invoke-Step {
    param([string]$Title, [scriptblock]$Body)
    Write-Host ""
    Write-Host "==> $Title" -ForegroundColor Cyan
    & $Body
}

function Write-TextNoBom {
    param([string]$Path, [string]$Text)
    [System.IO.File]::WriteAllText($Path, $Text, (New-Object System.Text.UTF8Encoding($false)))
}

if (-not (Test-Path "go.mod")) {
    throw "go.mod not found. Run this script from the repository scripts/ directory."
}

$RepoUrl = $RepoUrl.Trim()
if ($RepoUrl -notmatch '^https://github\.com/(?<owner>[^/]+)/(?<repo>[^/]+?)(\.git)?$') {
    throw "RepoUrl must look like https://github.com/<owner>/<repo>.git"
}
$Owner = $Matches['owner']
$Repo = $Matches['repo']
$NewModule = "github.com/$Owner/$Repo"

if (-not $SkipVerify) {
    Invoke-Step "Run tests, vet, and build" {
        & "$PSScriptRoot\test.ps1"
    }
}

$CurrentModule = (Select-String -Path "go.mod" -Pattern '^module\s+(\S+)').Matches[0].Groups[1].Value
if ($CurrentModule -ne $NewModule) {
    Invoke-Step "Update Go module path: $CurrentModule -> $NewModule" {
        $goMod = Get-Content "go.mod" -Raw
        Write-TextNoBom "go.mod" ($goMod -replace [regex]::Escape($CurrentModule), $NewModule)
        Get-ChildItem -Recurse -Filter *.go |
            ForEach-Object {
                $text = Get-Content $_.FullName -Raw
                if ($text -match [regex]::Escape($CurrentModule)) {
                    Write-TextNoBom $_.FullName ($text -replace [regex]::Escape($CurrentModule), $NewModule)
                    Write-Host "  updated $($_.FullName.Replace((Get-Location).Path + '\', ''))"
                }
            }
    }

    if (-not $SkipVerify) {
        Invoke-Step "Re-run tests after module rename" {
            & "$PSScriptRoot\test.ps1"
        }
    }
}

Invoke-Step "Stage and commit" {
    git add -A
    git diff --cached --quiet
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  nothing to commit"
    }
    else {
        git commit -m $CommitMessage
    }
    git branch -M main
}

Invoke-Step "Configure remote origin" {
    $existing = git remote
    if ($existing -contains "origin") {
        git remote set-url origin $RepoUrl
    }
    else {
        git remote add origin $RepoUrl
    }
    git remote -v
}

Invoke-Step "Push main" {
    git push -u origin main
}

if (-not $SkipTag) {
    Invoke-Step "Tag and push $Version" {
        git tag $Version
        git push origin $Version
    }
    Write-Host ""
    Write-Host "Release workflow triggered. Watch it at:" -ForegroundColor Green
    Write-Host "  https://github.com/$Owner/$Repo/actions"
}
else {
    Write-Host ""
    Write-Host "Skipped tagging. To cut a release later:" -ForegroundColor Yellow
    Write-Host "  git tag $Version; git push origin $Version"
}
