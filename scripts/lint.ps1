# Runs the same pinned linter image and configuration as CI.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
& docker run --rm --mount "type=bind,source=$projectRoot,target=/src,readonly" --mount 'type=volume,source=backend-go-final-lint-modcache,target=/go/pkg/mod' --mount 'type=volume,source=backend-go-final-lint-cache,target=/root/.cache/golangci-lint' -w /src golangci/golangci-lint:v2.15.0 golangci-lint run --config=.golangci.yml ./ads/... ./common/...
if ($LASTEXITCODE -ne 0) { throw "Lint failed with exit code $LASTEXITCODE." }
