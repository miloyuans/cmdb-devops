# CMDB DevOps

CMDB DevOps 是一个 Go + MongoDB 实现的多云 CMDB / DevOps 资产查询平台。它按主动采集型架构设计：后台周期拉取云资源、身份用户、AccessKey 状态并写入 MongoDB；用户通过 Web UI 或 Telegram Bot 查询时优先读取缓存，并在缓存未命中时触发带防抖的异步刷新任务。

> 当前包提供可运行生产骨架和完整业务闭环。默认 provider 为 `mock`，可以立即跑通账户配置、地区发现、资源同步、IP 查询、通信分析、身份/AK 反查、Telegram 配置。真实 AWS / 阿里云采集器可以按 `internal/cloud.Provider` 接口替换或扩展。

## 功能

- 紧凑型 Web Console：左侧功能导航、右上角登录/退出、右侧列表 + 详情窗口
- 普通用户 viewer / 管理员 admin
- 云账户 Web 配置，AK/SK 加密保存
- 地区自动发现：默认自动发现有资源地区，也支持手动选择地区
- 地区发现任务：每天一次，可 UI 手动触发；运行中后端锁防止重复触发
- 资源采集任务：默认 30 分钟一次，仅遍历 effective_regions
- MongoDB 每账户独立库
- IP / CIDR 查询：IPv4 / IPv6、内网 / 公网判断，多库汇总去重
- 资源通信分析：基于私网地址、VPC、子网、安全组规则做缓存侧判断
- 身份与密钥治理：IAM/RAM 用户、AK 列表、AK 反查、最近使用时间字段
- Telegram 配置通过 Web UI 管理，支持多个 Bot、多个群、允许交互用户白名单，默认未配置白名单时允许所有用户
- 查询功能合并 IP/CIDR 与 AK 反查，通过子窗口切换
- 审计事件独立页面
- 系统设置独立页面：Web 页面名称、异步同步间隔、Mongo 配置展示
- 平台用户管理独立页面：普通用户 viewer / 管理用户 admin，用户可更新自己的密码和 Telegram ID
- 查询 miss 触发异步刷新，5 分钟内防抖合并


## 构建前自检

如果遇到 Docker build 路径问题，先执行：

```bash
./scripts/check-build-structure.sh .
```

应输出：

```text
build structure check ok
```

关键入口文件必须存在：

```text
cmd/cmdb-devops/main.go
```

## 快速启动

推荐在项目根目录启动，避免 Docker build context 指向错误：

```bash
docker compose up --build
```

兼容旧方式：

```bash
cd deployments
docker compose up --build
```

本版本已经修复 Dockerfile 的构建路径问题：构建时会自动识别 `/src/go.mod` 或 `/src/cmdb-devops/go.mod`，并在找不到 `cmd/cmdb-devops` 时输出目录诊断。

浏览器打开：

```text
http://localhost:8080
```

默认登录：

```text
admin / admin123456
```

建议生产环境通过环境变量修改：

```text
DEFAULT_ADMIN_USER
DEFAULT_ADMIN_PASSWORD
JWT_SECRET
ENC_KEY
```

`ENC_KEY` 建议使用 32 字节随机字符串，用于加密云账户 AK Secret 和 Telegram Bot Token。

## 本地启动

需要本机已启动 MongoDB：

```bash
cp .env.example .env
./scripts/run-local.sh
```

## 典型使用流程

1. 登录 Web Console。
2. 进入“云账户管理”，新增 AWS 或阿里云账户。
3. 点击“检测地区”，系统异步写入 detected_regions 和 effective_regions。
4. 点击“同步资源”，系统把 mock provider 的资源写到账户独立 Mongo 库。
5. 进入“IP 查询”，输入：

```text
10.0.1.12
```

6. 进入“身份与密钥”，点击“刷新列表”或先触发身份同步，再输入 mock AK 反查。

mock AWS AK 形态：

```text
AKIA<account_id>MOCK
```

mock 阿里云 AK 形态：

```text
LTAI<account_id>MOCK
```

## 真实云厂商接入点

核心接口在：

```text
internal/cloud/provider.go
```

```go
type Provider interface {
    Name() string
    ValidateAccount(ctx context.Context, account model.CloudAccount, secret string) error
    DiscoverRegions(ctx context.Context, account model.CloudAccount, secret string) ([]model.RegionInfo, error)
    CollectInventory(ctx context.Context, account model.CloudAccount, secret string, regions []string) (*InventorySnapshot, error)
    CollectIdentity(ctx context.Context, account model.CloudAccount, secret string) (*IdentitySnapshot, error)
}
```

默认实现：

```text
internal/cloud/mock.go
```

生产接入建议新增：

```text
internal/cloud/aws/provider.go
internal/cloud/aliyun/provider.go
```

其中：

- `DiscoverRegions`：先查询可用地域，再使用事件历史 + 轻量 Describe API 判断有资源地区。
- `CollectInventory`：按 account + effective_regions 拉取 EC2/ECS、ENI、VPC、Subnet/VSwitch、安全组、路由表、NAT、LB 等资源。
- `CollectIdentity`：拉取 IAM/RAM 用户、策略、AK、AK 最近使用时间。

## MongoDB 结构

管理库：

```text
cmdb_admin
  users
  cloud_accounts
  jobs
  audit_logs
  system_settings
  access_key_global_index
  telegram_bots
  telegram_chats
  telegram_users
  telegram_config       # 兼容旧版本
  telegram_sessions
```

账户独立库：

```text
cmdb_<provider>_<alias>
  resources
  ip_index
  security_group_rules
  resource_edges
  iam_users
  access_keys
```

## API 摘要

```text
POST /api/login
GET  /api/me
PUT  /api/me/profile
GET  /api/cloud/platforms
GET  /api/accounts
GET  /api/accounts/:id
POST /api/accounts
PUT  /api/accounts/:id
POST /api/accounts/:id/jobs/region_discovery
POST /api/accounts/:id/jobs/inventory_sync
POST /api/accounts/:id/jobs/identity_sync
POST /api/query/ip
POST /api/query/connectivity
GET  /api/identity/users
GET  /api/identity/access-keys
POST /api/identity/access-keys/lookup
GET  /api/telegram/bots
POST /api/telegram/bots
PUT  /api/telegram/bots/:id
GET  /api/telegram/chats
POST /api/telegram/chats
PUT  /api/telegram/chats/:id
GET  /api/telegram/users
POST /api/telegram/users
PUT  /api/telegram/users/:id
GET  /api/settings
PUT  /api/settings
GET  /api/users
POST /api/users
PUT  /api/users/:id
GET  /api/audit
POST /api/telegram/webhook
```

## 安全建议

- 不要保存 AccessKeySecret 明文。
- 生产环境使用 Vault/KMS 替代本地 AES key。
- 给 CMDB DevOps 的云端 AK 使用最小只读权限。
- Web 入口必须使用 HTTPS。
- Telegram Webhook URL 必须使用 HTTPS。
- 真实 provider 中所有 API 调用必须加 context timeout、重试、限流和审计。

## 下一步生产增强

- 替换 mock provider 为真实 AWS / 阿里云 provider。
- 将 `ip_index` 的 CIDR 查询升级为 IPv4/IPv6 数值区间索引。
- 增加 NACL、RouteTable、VPC Peering、TGW、CEN 的完整路由判断。
- 增加 Casbin 权限策略。
- 增加通知队列 `telegram_notification_events` 的可靠消费 worker。


## v0.2 修复与增强

- 修复 Docker build `stat /src/cmd/cmdb-devops: directory not found`：Dockerfile 改为自动识别工程根目录并输出诊断。
- 新增根目录 `compose.yaml`，推荐直接在项目根目录执行 `docker compose up --build`。
- IP / CIDR 查询改为多账户并发查询 Mongo 缓存，避免串行遍历账户库。
- 通信分析同时检查源安全组出方向和目标安全组入方向，不再只看目标 ingress。
- 身份与密钥模块新增 `GET /api/identity/users`，Web UI 可展示 IAM/RAM 用户、用户组和策略摘要。
- AK 反查接口返回 `found/access_key/owner_user` 结构，未命中时会触发带防抖的 `identity_sync`。
- 身份同步时会清理该账户旧的全局 AK 索引，避免已删除 AK 长期残留。

## 构建故障处理

如果 Docker 构建阶段出现 `missing go.sum entry`，说明当前包里的依赖校验文件尚未生成，v0.4 的 Dockerfile 已在容器内执行：

```bash
go mod tidy
go mod download
go build -mod=mod ./cmd/cmdb-devops
```

本地调试时也可以执行：

```bash
./scripts/local-build-check.sh
```

如果你所在环境使用私有 Go 代理，可以在构建前设置：

```bash
export GOPROXY=https://goproxy.cn,direct
```

## v0.5 UI 与管理功能升级

- 云账户管理改为左侧列表、右侧详情编辑模式。
- 云账户默认平台选择固定为 AWS / 阿里云。
- Telegram 管理拆分为 Bot、群、用户三类资源。
- Telegram 支持多 Bot，多群启用/禁用，允许交互用户白名单；未配置白名单时默认允许所有用户。
- 查询页面合并 IP/CIDR 查询与 AK 反查，使用子窗口切换。
- 登录用户信息与退出放到右上角。
- 新增审计事件页面。
- 新增系统设置页面。
- 新增平台用户管理页面，支持 viewer/admin 两类权限。
- 新增个人资料更新接口，允许用户修改自己的密码和 Telegram ID。
- Mongo 初始化增加 users、cloud_accounts、jobs、system_settings、audit_logs、telegram_bots、telegram_chats、telegram_users 等索引。
