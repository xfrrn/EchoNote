# Phase 9：Identity 与请求安全实施记录

更新时间：2026-08-21

## 范围

本阶段完成：

- `users`、`sessions`、既有 `user_id` 回填与外键。
- 用户名/密码登录、数据库 Session、退出和当前用户 API。
- 请求级用户上下文，替换 Server 固定用户字段。
- 修改请求同源 Origin 校验、认证响应 `no-store`。
- 管理员创建用户、认领占位用户、重置密码和撤销 Session。
- development / test 固定身份兼容；staging / production 明确禁止。

没有实现公共注册、邮箱找回、OAuth、JWT、Redis Session 或独立 Auth 服务。

## 实现前分析

```text
当前代码
  Config 在启动时读取 ECHONOTE_USER_ID
  Server 把该 UUID 保存为共享字段
  所有 Handler 把同一个 UUID 传给已有 Repository

目标
  Cookie 明文 Token → SHA-256 → sessions
  sessions → active user → request context
  Handler 从 request context 取得 user_id
  已有 Repository 隔离查询保持不变
```

主要修改文件：

- `migrations/000007_identity.*.sql`
- `internal/database/queries/auth.sql` 与 sqlc 生成物
- `internal/auth`、`internal/repository/auth.go`
- `internal/http/auth.go` 与所有业务 Handler 的用户来源
- `cmd/admin`、`cmd/api`、Config、OpenAPI

主要风险与处理：

- 旧业务数据没有用户表：Migration 先汇总所有业务表的非空 `user_id`，回填 `placeholder` 用户，再验证外键。
- Session Token 泄漏：只在 Cookie 中发送一次，数据库只存 32-byte SHA-256，日志不读取 Header。
- 用户枚举：不存在用户和错误密码返回完全相同的状态与 JSON，并执行同成本的假密码校验。
- CSRF：staging / production 要求唯一 HTTPS `PUBLIC_ORIGIN`，所有非安全方法必须精确匹配 `Origin`。
- 开发升级：仅 development / test 可继续使用 `ECHONOTE_USER_ID`，API 启动时只补不可登录占位用户。

## 数据模型与迁移

Migration v7 新增：

- `users`：原始用户名、NFKC + lowercase 规范名、bcrypt Hash、`placeholder | active | disabled` 状态和时间戳。
- `sessions`：用户、Token 摘要、明确过期、撤销、创建和最后访问时间。

所有既有直接含 `user_id` 的业务表都添加到 `users(id)` 的外键；`jobs.user_id` 继续允许为空。旧 ID 不重写，管理员使用：

```bash
go run ./cmd/admin claim <existing-user-id> <username>
```

升级测试从 Migration v6 插入既有 Episode，再升级 v7，验证占位用户、认领和原 Episode ID 均保持正确。

## 密码与 Session

用户名允许 Unicode 字母/数字及 `._-`，长度 3–64；规范化使用 NFKC 后 lowercase，数据库以规范名唯一。

密码为 12–72 UTF-8 bytes，使用 bcrypt。生产参数不写死为不可调常量：目标主机运行 `admin benchmark-password` 后设置 `PASSWORD_BCRYPT_COST`，配置只接受 10–16。

Session Token 使用 `crypto/rand` 生成 256 bits，Cookie 属性为：

```text
Secure（staging / production）
HttpOnly
SameSite=Lax
Path=/
无 Domain
Expires + Max-Age
```

每次请求按 Token 摘要查询未撤销、未过期且用户仍为 active 的 Session。API 或 Worker 重启不会改变数据库 Session 或 Job。

## API 与中间件

OpenAPI 0.9.0 新增：

```text
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/me
```

全局声明 Cookie Security Scheme；只有登录和健康检查公开。认证中间件在共享路由入口完成身份解析，业务 Handler 不再保存或读取固定用户字段。

所有 `/api/` 响应设置 `Cache-Control: no-store`。请求日志只记录 `request_id`、解析后的 `user_id`、方法、路径、状态和耗时，不记录密码、Token 或 Cookie。

## 管理入口

```text
admin create <username>
admin claim <user-id> <username>
admin reset-password <username>
admin benchmark-password
```

密码没有命令参数或环境变量入口：TTY 使用无回显读取，管道使用标准输入第一行。密码重置和全部 Session 撤销在同一数据库事务完成。

## 验收

自动验收覆盖：

- 未认证业务 API 返回 401。
- 登录 Cookie 属性、`/me`、新 Router 实例保持 Session。
- 退出、过期、密码重置撤销后立即 401。
- 用户不存在和密码错误响应一致。
- 非同源修改请求返回 403。
- 日志不出现密码、Token 或 Cookie。
- 既有跨用户 Episode、Note、Transcript、Search、AI 和 Export 隔离回归。
- Migration v6 → v7 旧用户回填和认领。
- 独立 PostgreSQL 从 0 Migration、`go test ./...`、`go vet ./...` 和生成物检查。

本阶段发现并修复了一个共享 Job Queue 时钟问题：未指定 `run_after` 时改由 PostgreSQL `now()` 设置，避免应用与数据库时钟相差数毫秒导致新 Job 暂时不可领取。

## 回滚

v7 是加法 Migration。生产回滚只切回兼容二进制，保留 `users`、`sessions` 和外键，不恢复固定用户模式，不执行 Down Migration。Down 文件只供隔离开发/测试数据库验证迁移边界。
