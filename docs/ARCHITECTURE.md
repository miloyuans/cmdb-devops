# CMDB DevOps Architecture

## Boundary

CMDB DevOps is a cache-first, active-pull CMDB. Cloud APIs are called by background jobs only:

- `region_discovery`: detects effective regions for an account.
- `inventory_sync`: pulls cloud resources for effective regions.
- `identity_sync`: pulls users, roles/policies and AccessKeys.
- `miss_refresh`: triggered by query miss and merged by a 5-minute lock window.

User queries never scan cloud APIs directly. They read MongoDB indexes.

## Region discovery

For a newly configured account:

1. If `region_mode=manual`, `effective_regions=selected_regions`.
2. If `region_mode=auto`, a `region_discovery` job fills `detected_regions`.
3. `effective_regions` is built from detected regions where `has_resource=true`.
4. Daily region discovery updates the account. Manual UI trigger uses a backend running-job lock.

Production provider implementation should combine:

- region list API, such as AWS `DescribeRegions` or Alibaba Cloud ECS `DescribeRegions`;
- event history as a fast signal;
- lightweight read-only Describe calls to confirm resource existence.

Do not rely only on event history, because it can miss old but still-running resources.

## Query model

Important collections per account DB:

- `resources`: normalized cloud resources.
- `ip_index`: direct IP/CIDR reverse lookup index.
- `security_group_rules`: normalized allow/deny policy rules.
- `resource_edges`: resource graph edges.
- `iam_users`: normalized IAM/RAM users.
- `access_keys`: normalized AccessKey records.

Global collections in `cmdb_admin`:

- `cloud_accounts`
- `users`
- `jobs`
- `access_key_global_index`
- `telegram_config`
- `telegram_chats`
- `telegram_sessions`

## Provider implementation checklist

For AWS:

- Region discovery: EC2 DescribeRegions, CloudTrail LookupEvents, EC2/ELB/NAT lightweight Describe.
- Inventory: EC2 Instances, ENI, VPC, Subnet, Security Groups, Route Tables, NAT Gateways, EIP, ALB/NLB/ELB, Target Groups.
- Identity: IAM ListUsers, ListAccessKeys, GetAccessKeyLastUsed, ListGroupsForUser, ListAttachedUserPolicies, ListUserPolicies, ListRoles.

For Alibaba Cloud:

- Region discovery: ECS DescribeRegions, ActionTrail events, ECS/VPC/SLB lightweight Describe.
- Inventory: ECS, ENI, EIP, VPC, VSwitch, Security Groups, Route Tables, NAT Gateway, SLB/ALB/NLB.
- Identity: RAM ListUsers, ListAccessKeys, GetAccessKeyLastUsed, ListPoliciesForUser, ListRoles.

## Security group communication analysis

The query engine evaluates:

1. source and target lookup from `ip_index`;
2. same account/region/VPC as the first route-level decision;
3. target ingress security group rules;
4. later extension: egress, NACL, route tables, peering, TGW and CEN.

## Telegram

Telegram configuration is stored in MongoDB and managed via Web UI.

- Bot token can be encrypted in MongoDB or read from an environment variable.
- Conversation sessions are stored in MongoDB and expire automatically.
- `/list` queries IP/CIDR.
- `/ak` reverse-lookups AccessKeyId.

Production enhancement: implement `telegram_notification_events` queue with lease-based workers for reliable multi-replica delivery.
