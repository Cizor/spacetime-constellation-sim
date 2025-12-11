# Script to start working on a GitHub issue
# Usage: .\start-issue.ps1 -IssueNumber 120

param(
    [Parameter(Mandatory=$true)]
    [int]$IssueNumber,
    
    [string]$BaseBranch = "main",
    [string]$GitHubToken = $env:GITHUB_TOKEN
)

$ErrorActionPreference = "Stop"

# Get GitHub token from environment variable or parameter
if (-not $GitHubToken) {
    Write-Host "Error: GitHub token not provided. Set GITHUB_TOKEN environment variable or use -GitHubToken parameter." -ForegroundColor Red
    Write-Host "Example: `$env:GITHUB_TOKEN = 'your_token'; .\start-issue.ps1 -IssueNumber 120" -ForegroundColor Yellow
    exit 1
}

# Fetch issue details
$token = $GitHubToken
$repo = "Cizor/spacetime-constellation-sim"
$baseUrl = "https://api.github.com/repos/$repo/issues"
$headers = @{
    "Authorization" = "token $token"
    "Accept" = "application/vnd.github.v3+json"
}

Write-Host "Fetching issue #$IssueNumber..." -ForegroundColor Cyan
try {
    $issue = Invoke-RestMethod -Uri "$baseUrl/$IssueNumber" -Headers $headers
} catch {
    Write-Host "Error fetching issue: $_" -ForegroundColor Red
    exit 1
}

Write-Host "Issue: $($issue.title)" -ForegroundColor Green
Write-Host "State: $($issue.state)" -ForegroundColor $(if ($issue.state -eq "open") { "Green" } else { "Yellow" })

if ($issue.state -ne "open") {
    Write-Host "Warning: Issue is not open. Continue? (y/n)" -ForegroundColor Yellow
    $response = Read-Host
    if ($response -ne "y") {
        exit 1
    }
}

# Extract chunk number
$chunk = "?"
if ($issue.title -match "Chunk (\d+)") {
    $chunk = $matches[1]
}

# Generate branch name
$slug = ($issue.title -replace "^\[Scope 4\]\[Chunk \d+\]\s*", "" -replace "[^a-zA-Z0-9\s-]", "" -replace "\s+", "-" -replace "-+", "-").ToLower()
$slug = $slug.Substring(0, [Math]::Min(30, $slug.Length))
$branchName = "issue-$IssueNumber-$slug"

Write-Host "`nBranch name: $branchName" -ForegroundColor Cyan

# Check current branch
$currentBranch = git branch --show-current
if ($currentBranch -ne $BaseBranch) {
    Write-Host "Current branch is '$currentBranch', not '$BaseBranch'. Switch? (y/n)" -ForegroundColor Yellow
    $response = Read-Host
    if ($response -eq "y") {
        git checkout $BaseBranch
    } else {
        Write-Host "Aborted." -ForegroundColor Red
        exit 1
    }
}

# Pull latest
Write-Host "`nPulling latest from $BaseBranch..." -ForegroundColor Cyan
git pull origin $BaseBranch

# Check if branch exists
if (git branch --list $branchName) {
    Write-Host "Branch '$branchName' already exists. Use it? (y/n)" -ForegroundColor Yellow
    $response = Read-Host
    if ($response -eq "y") {
        git checkout $branchName
        Write-Host "Switched to existing branch." -ForegroundColor Green
        exit 0
    } else {
        Write-Host "Aborted." -ForegroundColor Red
        exit 1
    }
}

# Create and checkout branch
Write-Host "Creating branch '$branchName'..." -ForegroundColor Cyan
git checkout -b $branchName

Write-Host "`n✅ Ready to work on issue #$IssueNumber" -ForegroundColor Green
Write-Host "Branch: $branchName" -ForegroundColor Cyan
Write-Host "Issue: $($issue.html_url)" -ForegroundColor Cyan

# Create initial commit structure (optional)
Write-Host "`nCreate initial commit structure? (y/n)" -ForegroundColor Yellow
$response = Read-Host
if ($response -eq "y") {
    $commitMsg = "[Scope 4][Chunk $chunk] Start work on issue #$IssueNumber`n`n$($issue.title)`n`nFixes #$IssueNumber"
    git commit --allow-empty -m $commitMsg
    Write-Host "Created initial commit." -ForegroundColor Green
}

