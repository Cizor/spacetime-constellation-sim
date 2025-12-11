Add-Type -AssemblyName System.Drawing
$pdfPath = $args[0]
try {
    # Try using iTextSharp or similar - but we need to check what's available
    Write-Host "Attempting to read PDF structure..."
    $bytes = [System.IO.File]::ReadAllBytes($pdfPath)
    Write-Host "PDF file size: $($bytes.Length) bytes"
    Write-Host "First 100 bytes (hex):"
    $bytes[0..99] | ForEach-Object { "{0:X2}" -f $_ } | Out-String
} catch {
    Write-Host "Error: $_"
}
