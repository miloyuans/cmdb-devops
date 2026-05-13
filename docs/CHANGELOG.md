# Changelog

## v0.2

### Fixed

- Docker build no longer hard-codes a fragile build context. The Dockerfile detects the project root and validates `cmd/cmdb-devops` before building.
- Added root-level `compose.yaml`; run `docker compose up --build` from the project root.

### Improved

- IP/CIDR query now scans account databases concurrently with a bounded worker limit.
- Connectivity analysis now checks both source egress security group rules and target ingress security group rules.
- Identity module exposes IAM/RAM users with group and attached policy summaries.
- AK lookup returns the owning user document when available.
- Identity sync removes stale global AK index rows for the account before writing fresh keys.

### Notes

- The default provider remains `mock` so the whole service can run without real cloud credentials.
- Replace `internal/cloud/mock.go` with AWS/Aliyun provider implementations that satisfy `cloud.Provider` for production cloud collection.
