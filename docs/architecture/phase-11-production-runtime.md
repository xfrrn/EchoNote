# Phase 11：生产运行环境实施记录

- 状态：仓库实现与 Ubuntu 24.04 WSL 原生部署验收完成；独立 Staging 主机、真实证书与外部告警待执行
- 日期：2026-08-21

## 当前代码 → 目标

`deployments` 原先只有占位文件，API 监听所有接口，生产配置允许 Provider 禁用，数据库 Role、TLS、连接预算和 Worker 残留临时文件没有启动门禁。目标是提供可重复的原生 Linux 部署，并让错误生产配置在服务启动前失败。

## 修改文件

- `internal/config`、`cmd/api`、`cmd/worker`：生产数据库必须为预期 EchoNote 库，使用非 `postgres` Role、`sslmode=verify-full` 和显式连接池预算；API 只监听回环；API/Worker 分别验证所需 Provider；支持 `--check-config`。
- `internal/database/postgres.go`：运行时拒绝超级用户、建库/建 Role、绕过 RLS、数据库或 Schema CREATE 权限，并验证 `pg_trgm`、`vector`。
- `internal/service`：Worker 启动清理超过安全时限的 EchoNote 临时常规文件，跳过目录、符号链接和其他文件。
- `cmd/admin`、Job Queue：只允许人工重排失败的 `cleanup_audio` Job，并记录可审计 Event。
- `deployments/nginx`：HTTPS、同域 API、SSE 无缓冲、静态缓存、安全头和登录/高成本写限速。
- `deployments/systemd`：API、Worker、Migration 与 Maintenance cleanup retry 的独立非 root、只读文件系统单元。
- `deployments/env`、`postgres`、`runbooks`、`scripts/smoke.sh`：无真实密钥的分离模板、最小权限授权、Worker 身份表隔离、部署/迁移/回滚/维护和发布后 Smoke。

## 风险与边界

- Ubuntu 24.04 WSL 已覆盖原生二进制、Nginx、systemd、SSE、Lease 和进程重启，但不能替代全新独立 Staging 主机、真实证书续期、外部告警投递、主机重启和第二位工程师复现。
- 临时文件 age fence 以单 Worker 主机为首版边界；横向扩展前改为每 Worker 独立目录。
- 数据库 Role 创建、Bucket 私有策略、云端额度和告警由目标平台实施，仓库只提供强制启动检查与 Runbook。
- 常规回滚只切换不可变 Web/二进制 Release，不执行 Production Down Migration。

## 本地验收证据

- Ubuntu 24.04 WSL 从仓库根目录构建 5 个 `linux/amd64`、`CGO_ENABLED=0` 二进制，安装不可变 Release、4 个非 root 服务用户、Nginx 与 systemd Unit；API/Worker `--check-config`、`nginx -t`、`systemd-analyze verify` 通过，API/Worker 的 `systemd-analyze security` exposure 均为 3.1。
- PostgreSQL 17 + pgvector 通过 `sslmode=verify-full` 从 0 迁移到版本 8、`dirty=false`。原生演练发现并修复了 Runbook 构建目录、数据库 CA 目录穿越权限，以及 `pool_max_conns` 被迁移驱动误传为 PostgreSQL GUC 三个发布阻断问题。
- API、Worker 与 Nginx 分别重启后 PID 变化且 Session 仍有效；真实 SSE 返回 `started/completed`，`Last-Event-ID: 1` 只重放后一事件；Worker 进程启动回收过期 Lease，Job 以 attempt 2 完成。
- Release Symlink 从 `native-fix2` 回切 `native-fix1` 后配置预检、Nginx、readiness 与 HTTPS Smoke 通过，再前滚到 `native-fix2`；Migration 始终为版本 8，未执行 Down Migration。
- 独立 PostgreSQL 17 + pgvector：`go test -p 1 ./... -count=1` 通过；包级串行避免多个集成测试包争用同一临时任务队列。
- `go generate ./...` 与 TypeScript Contract 生成前后 SHA-256 一致，`go vet ./...`、`pnpm build`、`git diff --check` 通过。
- Nginx 1.24 官方镜像加载仓库配置并执行 `nginx -t` 通过；生产部署仍不依赖 Docker。
- 本地实验 API 使用 `127.0.0.1:18080`、HTTPS 使用自签证书；仓库 Production 默认端口和真实受信证书仍按 Runbook 在独立 Staging 复验。
- `smoke.sh` 通过 `bash -n`，外置 Theme 初始化脚本通过 Node 语法检查，生产 `index.html` 不含内联 Script。
