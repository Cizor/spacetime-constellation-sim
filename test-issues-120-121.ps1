# Test Issues #120 and #121 Locally
# This script checks out each branch, runs tests, and builds the code

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "Testing Scope 4 Issues #120 and #121" -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Green

# Save current branch
$originalBranch = git branch --show-current
Write-Host "Current branch: $originalBranch" -ForegroundColor Gray
Write-Host ""

# Function to test a branch
function Test-Branch {
    param(
        [string]$BranchName,
        [string]$IssueNumber,
        [string]$Description
    )
    
    Write-Host "`n========================================" -ForegroundColor Cyan
    Write-Host "Issue #$IssueNumber: $Description" -ForegroundColor Cyan
    Write-Host "Branch: $BranchName" -ForegroundColor Cyan
    Write-Host "========================================`n" -ForegroundColor Cyan
    
    # Checkout branch
    Write-Host "Checking out branch..." -ForegroundColor Yellow
    git checkout $BranchName 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to checkout branch $BranchName" -ForegroundColor Red
        return $false
    }
    
    # Run tests
    Write-Host "`nRunning tests..." -ForegroundColor Yellow
    if ($IssueNumber -eq "120") {
        $testOutput = go test ./internal/sim/state/... ./core/... -v -run "TestScenarioStateActivate|TestScenarioStateLinkStatus|TestWireless|TestMultiBeam|TestManualLinkImpairment" 2>&1
    } else {
        $testOutput = go test ./internal/sim/state/... -v -run "TestScenarioStateRoutingOperations" 2>&1
    }
    
    $testOutput | Select-Object -Last 10
    $testPassed = $LASTEXITCODE -eq 0
    
    if ($testPassed) {
        Write-Host "`n✓ Tests PASSED" -ForegroundColor Green
    } else {
        Write-Host "`n✗ Tests FAILED" -ForegroundColor Red
    }
    
    # Build
    Write-Host "`nBuilding code..." -ForegroundColor Yellow
    $buildOutput = go build ./... 2>&1
    $buildPassed = $LASTEXITCODE -eq 0
    
    if ($buildPassed) {
        Write-Host "✓ Build SUCCESS" -ForegroundColor Green
    } else {
        Write-Host "✗ Build FAILED" -ForegroundColor Red
        $buildOutput | Select-Object -Last 10
    }
    
    # Run all tests for the package
    Write-Host "`nRunning all package tests..." -ForegroundColor Yellow
    if ($IssueNumber -eq "120") {
        go test ./internal/sim/state/... ./core/... 2>&1 | Select-String -Pattern "(PASS|FAIL|ok)" | Select-Object -Last 5
    } else {
        go test ./internal/sim/state/... 2>&1 | Select-String -Pattern "(PASS|FAIL|ok)" | Select-Object -Last 5
    }
    
    return ($testPassed -and $buildPassed)
}

# Test Issue #120
$issue120Passed = Test-Branch -BranchName "issue-120-link-status-activation" -IssueNumber "120" -Description "Link Status and Activation Helpers"

# Test Issue #121
$issue121Passed = Test-Branch -BranchName "issue-121-routing-tables" -IssueNumber "121" -Description "Routing Tables and Helpers"

# Restore original branch
Write-Host "`n========================================" -ForegroundColor Green
Write-Host "Restoring original branch..." -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Green
git checkout $originalBranch 2>&1 | Out-Null

# Summary
Write-Host "`n========================================" -ForegroundColor Green
Write-Host "Test Summary" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "Issue #120 (Link Status): $(if ($issue120Passed) { 'PASSED ✓' } else { 'FAILED ✗' })" -ForegroundColor $(if ($issue120Passed) { 'Green' } else { 'Red' })
Write-Host "Issue #121 (Routing Tables): $(if ($issue121Passed) { 'PASSED ✓' } else { 'FAILED ✗' })" -ForegroundColor $(if ($issue121Passed) { 'Green' } else { 'Red' })
Write-Host ""

if ($issue120Passed -and $issue121Passed) {
    Write-Host "All tests passed! ✓" -ForegroundColor Green
    exit 0
} else {
    Write-Host "Some tests failed. Please review the output above." -ForegroundColor Red
    exit 1
}



