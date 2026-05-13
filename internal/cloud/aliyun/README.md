# Alibaba Cloud provider adapter

Implement this directory when replacing mock provider with live Alibaba Cloud SDK for Go V2.

Recommended packages vary by service, for example:

- ECS SDK for ECS, ENI and security groups
- VPC SDK for VPC, VSwitch, route tables, NAT Gateway and EIP
- RAM SDK for users, roles, policies and AccessKeys
- ActionTrail SDK or OpenAPI generic call for event history

Expose a type that satisfies `cloud.Provider`, then register it in `internal/app/app.go`.
