#!/usr/bin/env sh
set -eu
export $(grep -v '^#' .env.example | xargs)
go run ./cmd/cmdb-devops
