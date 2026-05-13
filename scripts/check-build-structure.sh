#!/usr/bin/env sh
set -eu
root="${1:-.}"
missing=0
for p in go.mod Dockerfile compose.yaml cmd/cmdb-devops/main.go internal/app/app.go internal/httpapi/router.go web/static/index.html; do
  if [ ! -e "$root/$p" ]; then
    echo "missing: $p"
    missing=1
  fi
done
if [ "$missing" -ne 0 ]; then
  echo "build structure check failed"
  exit 1
fi
echo "build structure check ok"
