# NPS-DSS Deep Mesh Test Suite
$ErrorActionPreference = "Continue"
Write-Host "--- STARTING DEEP MESH AUDIT ---" -ForegroundColor Cyan

# 1. CLEANUP & DEPLOY
Write-Host "[1/5] Resetting Cluster..." -ForegroundColor Yellow
docker-compose down
Remove-Item -Recurse -Force data, recovered_media -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force recovered_media > $null
docker-compose up -d --build
# Mesh readiness is now handled by the API Quorum Guard
# Wait a few seconds for the API Gateway itself to be reachable
Write-Host "Waiting for Gateway to boot..."
Start-Sleep -Seconds 5

# 2. AUTH
$AUTH_JSON = curl.exe -s -X POST -d "username=admin&password=adminpass" http://localhost:8080/v1/auth/login
Write-Host "Auth Response: $AUTH_JSON"
$TOKEN = ($AUTH_JSON | ConvertFrom-Json).token
if (!$TOKEN) { throw "Security Handshake Failed" }

# 3. MASS UPLOAD & SHARD AUDIT
Write-Host "[2/5] Mass Uploading Test Media..." -ForegroundColor Yellow
$testFiles = Get-ChildItem "test_media" -File
$fileManifests = @()

foreach ($file in $testFiles) {
    Write-Host " -> Distributing: $($file.Name)..."
    $upload_resp = curl.exe -s -X POST -H "Authorization: Bearer $TOKEN" -F "file=@$($file.FullName)" "http://localhost:8080/v1/files?key=$($file.Name)"
    Write-Host "    Response: $upload_resp"
    $resp = $upload_resp | ConvertFrom-Json
    $fileManifests += $resp
    Write-Host "    CID: $($resp.cid)" -ForegroundColor Green
}

# 4. DEDUPLICATION TEST
Write-Host "[3/5] Testing Content-Aware Deduplication..." -ForegroundColor Yellow
$dupFile = $testFiles[0] 
Write-Host " -> Re-uploading $($dupFile.Name) as 'clone_copy'..."
$dup_json = curl.exe -s -X POST -H "Authorization: Bearer $TOKEN" -F "file=@$($dupFile.FullName)" "http://localhost:8080/v1/files?key=clone_copy"
Write-Host "    Response: $dup_json"
$dupResp = $dup_json | ConvertFrom-Json
if ($dupResp.message -like "*deduplicated*") {
    Write-Host " SUCCESS: Deduplication confirmed" -ForegroundColor Green
} else {
    Write-Host " FAILURE: Deduplication failed" -ForegroundColor Red
}

# 5. DISASTER SIMULATION
Write-Host "[4/5] Inducing Network Fault (Stopping 2 nodes)..." -ForegroundColor Yellow
docker stop hydrastore_node1 hydrastore_node2

# 6. INTEGRITY VERIFICATION
Write-Host "[5/5] Reconstructing All Media from Mesh..." -ForegroundColor Yellow

# Function to find an active gateway
function Get-ActiveGateway {
    $ports = @(8080, 8081, 8082, 8083, 8084)
    foreach ($p in $ports) {
        $test = curl.exe -s -o /dev/null -w "%{http_code}" --max-time 2 "http://localhost:$p/v1/auth/login"
        if ($test -eq "405") { # POST required for login, so 405 means it's ALIVE
            return "http://localhost:$p"
        }
    }
    return $null
}

$ACTIVE_GATEWAY = Get-ActiveGateway
if (-not $ACTIVE_GATEWAY) {
    Write-Host " CRITICAL: No active gateways found in the mesh!" -ForegroundColor Red
    exit
}
Write-Host " -> Using Gateway: $ACTIVE_GATEWAY" -ForegroundColor Gray

foreach ($file in $testFiles) {
    Write-Host " -> Recovering $($file.Name)..."
    $target = "recovered_media\$($file.Name)"
    
    # Use the active gateway for recovery
    curl.exe -s -H "Authorization: Bearer $TOKEN" "$ACTIVE_GATEWAY/v1/files/$($file.Name)" --output $target
    
    if (Test-Path $target) {
        $recvSize = (Get-Item $target).Length
        if ($recvSize -eq $file.Length) {
            Write-Host "    RECOVERY SUCCESS: $($file.Name) is intact!" -ForegroundColor Green
        } else {
            Write-Host "    RECOVERY FAILED: Size mismatch for $($file.Name) ($($file.Length) vs $recvSize)" -ForegroundColor Red
        }
    } else {
        Write-Host "    RECOVERY FAILED: File $($file.Name) not found in mesh!" -ForegroundColor Red
    }
}

Write-Host "`n--- DEEP AUDIT COMPLETE ---" -ForegroundColor Cyan
