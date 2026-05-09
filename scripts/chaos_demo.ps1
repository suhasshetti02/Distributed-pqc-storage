$ErrorActionPreference = "Stop"

Write-Host "Phase 3 chaos demo (single machine)"
Write-Host "1) Start app normally and upload via /v1/files"
Write-Host "2) Stop one node process manually"
Write-Host "3) Download same file from gateway and verify success"
Write-Host "4) Open /metrics and /dashboard to show resilience counters"
