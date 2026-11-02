#!/bin/sh
set -eu
printf 'Checking formatting\n'
unformatted=$(gofmt -l ads common)
if [ -n "$unformatted" ]; then
    printf '%s\n' "$unformatted"
    exit 1
fi
printf 'Running go vet\n'
go vet ./...
printf 'Running unit, PostgreSQL and HTTP integration tests with race detection\n'
go test -v -race -tags=integration -count=1 ./...
printf 'Checking known vulnerabilities\n'
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
