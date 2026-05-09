param(
  [string]$ApiBase = "http://localhost:8080",
  [string]$ApiKey = "dev-api-key"
)

$ErrorActionPreference = "Stop"

Write-Host "Health check..."
Invoke-RestMethod -TimeoutSec 20 "$ApiBase/healthz" | Out-Null

$tmp = New-TemporaryFile
$bytes = New-Object byte[] 2048
(New-Object Random).NextBytes($bytes)
[System.IO.File]::WriteAllBytes($tmp.FullName, $bytes)

Write-Host "Upload check..."
Invoke-WebRequest -Method Post -TimeoutSec 30 -Uri "$ApiBase/v1/files?key=smoke.bin" -Headers @{ "X-API-Key" = $ApiKey } -InFile $tmp.FullName -ContentType "application/octet-stream" | Out-Null

Write-Host "List check..."
$list = Invoke-RestMethod -Method Get -TimeoutSec 20 -Uri "$ApiBase/v1/files" -Headers @{ "X-API-Key" = $ApiKey }
if (-not $list) { throw "list endpoint returned empty unexpectedly" }

Write-Host "Download check..."
$out = Join-Path $env:TEMP "smoke_download.bin"
Invoke-WebRequest -Method Get -TimeoutSec 30 -Uri "$ApiBase/v1/files/smoke.bin" -Headers @{ "X-API-Key" = $ApiKey } -OutFile $out

Write-Host "Metrics check..."
Invoke-RestMethod -TimeoutSec 20 "$ApiBase/metrics/json" | Out-Null

Remove-Item $tmp.FullName -Force
Remove-Item $out -Force
Write-Host "Smoke checks passed."
