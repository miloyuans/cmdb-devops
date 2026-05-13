# Changelog

## v0.3

- Fixed Docker build failure caused by missing `cmd/cmdb-devops` entrypoint directory.
- Added `cmd/cmdb-devops/main.go` with graceful shutdown and job scheduler startup.
- Added `Store.Close(ctx)` to cleanly disconnect MongoDB.
- Added `scripts/check-build-structure.sh` to verify package structure before building.

## v0.2

- Added concurrent multi-account IP query.
- Improved connectivity analysis for source egress and target ingress security group checks.
- Added identity user list endpoint and AccessKey owner lookup response.
- Cleaned stale global AccessKey index entries during identity sync.
