param(
    [switch]$RebuildApi
)

$ErrorActionPreference = "Stop"
$RootDir = Split-Path -Parent $PSScriptRoot
Set-Location $RootDir

Write-Host "[1/4] Pulling latest official RustDesk server image (hbbs & hbbr)..." -ForegroundColor Cyan
docker compose pull hbbs hbbr

if ($RebuildApi) {
    Write-Host "[2/4] Rebuilding custom rustdesk-api image..." -ForegroundColor Cyan
    docker compose build --pull rustdesk-api
} else {
    Write-Host "[2/4] Skipping rustdesk-api rebuild (use -RebuildApi to rebuild)." -ForegroundColor Gray
}

Write-Host "[3/4] Restarting containers..." -ForegroundColor Cyan
docker compose up -d --remove-orphans

Write-Host "[4/4] Checking service status..." -ForegroundColor Cyan
Start-Sleep -Seconds 2
docker compose ps
try {
    $health = Invoke-RestMethod -Uri "http://127.0.0.1:21114/health" -TimeoutSec 5
    Write-Host ("Health check OK: " + ($health | ConvertTo-Json -Compress)) -ForegroundColor Green
} catch {
    Write-Warning "Health check endpoint not yet responding on 127.0.0.1:21114."
}
