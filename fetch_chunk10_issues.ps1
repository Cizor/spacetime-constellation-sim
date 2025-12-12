# Script to fetch Chunk 10 issues from GitHub
# Usage: .\fetch_chunk10_issues.ps1 [-Token "optional_token"]

param(
    [string]$Token = $env:GITHUB_TOKEN
)

$repo = "Cizor/spacetime-constellation-sim"
$baseUrl = "https://api.github.com/repos/$repo/issues"

$headers = @{
    "Accept" = "application/vnd.github.v3+json"
}
if ($Token) {
    $headers["Authorization"] = "token $Token"
}

Write-Host "Fetching Chunk 10 issues..." -ForegroundColor Cyan

# Fetch all open issues and filter for Chunk 10
$allIssues = @()
$page = 1
$perPage = 100

do {
    $url = "${baseUrl}?state=open&per_page=$perPage&page=$page"
    
    try {
        $issues = Invoke-RestMethod -Uri $url -Headers $headers
        if ($issues.Count -eq 0) {
            break
        }
        $allIssues += $issues
        $count = $issues.Count
        Write-Host "Fetched page $page - $count issues" -ForegroundColor Green
        $page++
        Start-Sleep -Milliseconds 500
    } catch {
        Write-Host "Error fetching page $page - $_" -ForegroundColor Red
        break
    }
} while ($issues.Count -eq $perPage)

# Filter for Chunk 10 issues (by title or label)
$chunk10Issues = $allIssues | Where-Object { 
    $_.title -match "\[Scope 4\]\[Chunk 10\]" -or 
    $_.title -match "Chunk 10" -or
    ($_.labels | Where-Object { $_.name -match "chunk:10" -or $_.name -match "chunk-10" })
} | Sort-Object { $_.number }

$issueCount = $chunk10Issues.Count
Write-Host ""
Write-Host "Found $issueCount Chunk 10 issues" -ForegroundColor Cyan
foreach ($issue in $chunk10Issues) {
    $num = $issue.number
    $title = $issue.title
    Write-Host "  #$num - $title" -ForegroundColor Yellow
}

# Save to JSON
$chunk10Issues | ConvertTo-Json -Depth 10 | Out-File -FilePath "chunk10_issues.json" -Encoding utf8
Write-Host ""
Write-Host "Saved to chunk10_issues.json" -ForegroundColor Green

# Also create filtered version (just essential fields)
$filtered = $chunk10Issues | Select-Object number, title, state, html_url, body, labels
$filtered | ConvertTo-Json -Depth 10 | Out-File -FilePath "chunk10_issues_filtered.json" -Encoding utf8
Write-Host "Saved to chunk10_issues_filtered.json" -ForegroundColor Green
