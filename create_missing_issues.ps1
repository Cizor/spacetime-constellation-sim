# Script to create missing Scope 4 issues via GitHub API
# Usage: .\create_missing_issues.ps1 -Token "your_github_pat_token"

param(
    [Parameter(Mandatory=$true)]
    [string]$Token
)

$repo = "Cizor/spacetime-constellation-sim"
$baseUrl = "https://api.github.com/repos/$repo/issues"

$headers = @{
    "Authorization" = "token $Token"
    "Accept" = "application/vnd.github.v3+json"
}

# Read issues from JSON
$issues = Get-Content "missing_issues.json" | ConvertFrom-Json

foreach ($issue in $issues) {
    Write-Host "Creating issue: $($issue.title)"
    
    $body = @{
        title = $issue.title
        body = $issue.body
        labels = $issue.labels
    } | ConvertTo-Json -Depth 10
    
    try {
        $response = Invoke-RestMethod -Uri $baseUrl -Method Post -Headers $headers -Body $body -ContentType "application/json"
        Write-Host "  ✓ Created: #$($response.number) - $($response.html_url)" -ForegroundColor Green
        Start-Sleep -Milliseconds 500
    } catch {
        Write-Host "  ✗ Error: $_" -ForegroundColor Red
        $_.Exception.Response | Format-List
    }
}

Write-Host "`nDone! Created $($issues.Count) issues."




