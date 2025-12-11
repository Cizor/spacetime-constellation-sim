# Script to finish working on an issue: test, commit, push, create PR
# Usage: .\finish-issue.ps1 -IssueNumber 120 [-SkipTests] [-AutoMerge]

param(
    [Parameter(Mandatory=$true)]
    [int]$IssueNumber,
    
    [switch]$SkipTests,
    [switch]$AutoMerge,
    [string]$BaseBranch = "main",
    [string]$GitHubToken = $env:GITHUB_TOKEN
)

$ErrorActionPreference = "Stop"

# Check we're on a branch
$currentBranch = git branch --show-current
if ($currentBranch -eq $BaseBranch -or $currentBranch -eq "") {
    Write-Host "Error: Not on a feature branch. Current branch: $currentBranch" -ForegroundColor Red
    exit 1
}

Write-Host "Current branch: $currentBranch" -ForegroundColor Cyan

# Check for uncommitted changes
$status = git status --porcelain
if ($status) {
    Write-Host "`nUncommitted changes detected:" -ForegroundColor Yellow
    Write-Host $status
    Write-Host "`nCommit changes? (y/n)" -ForegroundColor Yellow
    $response = Read-Host
    if ($response -eq "y") {
        Write-Host "Enter commit message (or press Enter for auto-generated):" -ForegroundColor Yellow
        $commitMsg = Read-Host
        if (-not $commitMsg) {
            # Fetch issue for auto-generated message
            if (-not $GitHubToken) {
                Write-Host "Warning: GitHub token not provided. Using default commit message." -ForegroundColor Yellow
                $commitMsg = "Fixes #$IssueNumber"
            } else {
                $token = $GitHubToken
                $repo = "Cizor/spacetime-constellation-sim"
            $baseUrl = "https://api.github.com/repos/$repo/issues"
            $headers = @{
                "Authorization" = "token $token"
                "Accept" = "application/vnd.github.v3+json"
            }
            try {
                $issue = Invoke-RestMethod -Uri "$baseUrl/$IssueNumber" -Headers $headers
                $chunk = if ($issue.title -match "Chunk (\d+)") { $matches[1] } else { "?" }
                $commitMsg = "[Scope 4][Chunk $chunk] $($issue.title)`n`nFixes #$IssueNumber"
            } catch {
                $commitMsg = "Fixes #$IssueNumber"
            }
            }
        }
        git add -A
        git commit -m $commitMsg
        Write-Host "Committed changes." -ForegroundColor Green
    } else {
        Write-Host "Aborted." -ForegroundColor Red
        exit 1
    }
}

# Run tests
if (-not $SkipTests) {
    Write-Host "`nRunning tests..." -ForegroundColor Cyan
    go test ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Tests failed! Continue anyway? (y/n)" -ForegroundColor Red
        $response = Read-Host
        if ($response -ne "y") {
            exit 1
        }
    } else {
        Write-Host "All tests passed!" -ForegroundColor Green
    }
}

# Build check
Write-Host "`nBuilding..." -ForegroundColor Cyan
go build ./...
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed! Continue anyway? (y/n)" -ForegroundColor Red
    $response = Read-Host
    if ($response -ne "y") {
        exit 1
    }
} else {
    Write-Host "Build succeeded!" -ForegroundColor Green
}

# Push branch
Write-Host "`nPushing branch..." -ForegroundColor Cyan
git push -u origin $currentBranch
if ($LASTEXITCODE -ne 0) {
    Write-Host "Push failed!" -ForegroundColor Red
    exit 1
}

# Create PR
Write-Host "`nCreating PR..." -ForegroundColor Cyan
.\create-pr.ps1 -IssueNumber $IssueNumber -Branch $currentBranch -BaseBranch $BaseBranch -AutoMerge:$AutoMerge

Write-Host "`n✅ Issue #$IssueNumber ready for review!" -ForegroundColor Green




