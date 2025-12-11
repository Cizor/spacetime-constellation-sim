# Script to fetch and compare duplicate issues
# Usage: .\review_duplicates.ps1 [-Token "optional_token_for_auth"]

param(
    [string]$Token = ""
)

$repo = "Cizor/spacetime-constellation-sim"
$baseUrl = "https://api.github.com/repos/$repo/issues"

$headers = @{}
if ($Token) {
    $headers["Authorization"] = "token $Token"
    $headers["Accept"] = "application/vnd.github.v3+json"
}

$pairs = @(
    @(161, 162, "Chunk 8: Static route scheduling"),
    @(163, 164, "Chunk 8: ServiceRequest scheduling"),
    @(166, 167, "Chunk 9: Scenario startup"),
    @(168, 169, "Chunk 9: Run loop integration"),
    @(179, 180, "Chunk 12: Structured logging"),
    @(181, 182, "Chunk 12: Metrics counters")
)

foreach ($pair in $pairs) {
    $num1 = $pair[0]
    $num2 = $pair[1]
    $context = $pair[2]
    
    Write-Host "`n========================================"
    Write-Host "$context"
    Write-Host "========================================"
    
    try {
        $issue1 = Invoke-RestMethod -Uri "$baseUrl/$num1" -Headers $headers
        $issue2 = Invoke-RestMethod -Uri "$baseUrl/$num2" -Headers $headers
        
        Write-Host "`nIssue #${num1}:" -ForegroundColor Cyan
        Write-Host "  Title: $($issue1.title)"
        Write-Host "  State: $($issue1.state)"
        Write-Host "  Body Length: $($issue1.body.Length) chars"
        Write-Host "  URL: $($issue1.html_url)"
        
        Write-Host "`nIssue #${num2}:" -ForegroundColor Yellow
        Write-Host "  Title: $($issue2.title)"
        Write-Host "  State: $($issue2.state)"
        Write-Host "  Body Length: $($issue2.body.Length) chars"
        Write-Host "  URL: $($issue2.html_url)"
        
        # Compare
        if ($issue1.title -eq $issue2.title) {
            Write-Host "`n⚠️  EXACT DUPLICATE TITLES" -ForegroundColor Red
        } elseif ($issue1.body.Length -eq $issue2.body.Length -and $issue1.body -eq $issue2.body) {
            Write-Host "`n⚠️  EXACT DUPLICATE CONTENT" -ForegroundColor Red
        } else {
            Write-Host "`n📝 Titles differ - review descriptions" -ForegroundColor Yellow
        }
        
        Write-Host "`nRecommendation:"
        if ($issue1.body.Length -gt $issue2.body.Length) {
            Write-Host "  → Keep #${num1} (more detailed), consider closing #${num2}" -ForegroundColor Green
        } elseif ($issue2.body.Length -gt $issue1.body.Length) {
            Write-Host "  → Keep #${num2} (more detailed), consider closing #${num1}" -ForegroundColor Green
        } else {
            Write-Host "  → Review both descriptions to determine if duplicates or complementary" -ForegroundColor Yellow
        }
        
        Start-Sleep -Seconds 1  # Rate limit
        
    } catch {
        Write-Host "  ✗ Error fetching issues: $_" -ForegroundColor Red
        if ($_.Exception.Response.StatusCode -eq 403) {
            Write-Host "  → Rate limited. Wait a minute or provide a token." -ForegroundColor Yellow
        }
    }
}

Write-Host "`n`n========================================"
Write-Host "Review complete!"
Write-Host "========================================"

