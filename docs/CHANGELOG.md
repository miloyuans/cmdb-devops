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

## v0.5

- 重做 Web Console 信息架构：紧凑左侧导航、右上角账号菜单、右侧列表 + 详情布局。
- 云账户管理升级为账户列表 + 账户详情编辑；平台固定支持 AWS / 阿里云。
- Telegram 管理升级为 Bot / 群 / 用户三类配置资源，支持多 Bot、多群、启用/禁用、允许交互用户名单。
- 查询模块合并 IP/CIDR 查询与 AK 反查，通过子窗口切换。
- 新增审计事件模块。
- 新增系统设置模块，支持页面名称、异步同步间隔、Mongo 配置展示。
- 新增平台用户管理模块，支持普通用户 viewer 和管理用户 admin。
- 新增个人资料修改接口，用户可更新自己的密码和 Telegram ID。
- Mongo 初始化补充 audit_logs、system_settings、telegram_bots、telegram_users 等索引。
