#!/usr/bin/env sh
set -eu
./scripts/check-build-structure.sh .
go mod tidy
go test ./...
go build -o /tmp/cmdb-devops ./cmd/cmdb-devops
echo "local build check ok"
