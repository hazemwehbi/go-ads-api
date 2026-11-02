# Requires Docker Desktop with Linux containers; no host Go/C toolchain is needed.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Invoke-Docker {
    param([string[]]$DockerArgs)
    & docker @DockerArgs
    if ($LASTEXITCODE -ne 0) { throw "Docker verification failed with exit code $LASTEXITCODE." }
}
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    Get-Command docker -ErrorAction Stop | Out-Null
    Invoke-Docker -DockerArgs @('compose', '--profile', 'test', 'up', '-d', '--wait', 'test-database')
    # Pass a file path rather than shell source: Windows PowerShell 5.1 strips
    # embedded quotes from native executable arguments passed through sh -c.
    Invoke-Docker -DockerArgs @(
        'run', '--rm', '--network', 'backend-go-v2_default',
        '--mount', "type=bind,source=$projectRoot,target=/src,readonly",
        '--mount', 'type=volume,source=backend-go-v2-modcache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=backend-go-v2-buildcache,target=/root/.cache/go-build',
        '-w', '/src',
        '-e', 'TEST_DATABASE_URL=postgres://ads:ads@test-database:5432/ads_test?sslmode=disable',
        'golang:1.26.10', 'go', 'test', '-v', '-tags=integration,loadtest', '-count=1', '-timeout=90s', '-run', '^TestLimitedLoad$', './ads/internal/cmd/ads-api/dao/postgres'
    )
    Write-Host 'Limited load test passed.'
} finally {
    Pop-Location
}
