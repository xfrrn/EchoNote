# Phase 11：生产运行环境实施记录

- 状态：仓库实现完成，真实 Linux 主机与 HTTPS 验收待执行
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
- `deployments/env`、`postgres`、`runbooks`、`scripts/smoke.sh`：无真实密钥的分离模板、最小权限授权、部署/迁移/回滚/维护和发布后 Smoke。

## 风险与边界

- Nginx、systemd、证书续期、主机重启、SSE 和 Lease 恢复必须在真实 Staging Linux 主机验收；Windows 本地构建不能替代。
- 临时文件 age fence 以单 Worker 主机为首版边界；横向扩展前改为每 Worker 独立目录。
- 数据库 Role 创建、Bucket 私有策略、云端额度和告警由目标平台实施，仓库只提供强制启动检查与 Runbook。
- 常规回滚只切换不可变 Web/二进制 Release，不执行 Production Down Migration。

## 本地验收证据

- 独立 PostgreSQL 17 + pgvector：`go test -p 1 ./... -count=1` 通过；包级串行避免多个集成测试包争用同一临时任务队列。
- `go generate ./...` 与 TypeScript Contract 生成前后 SHA-256 一致，`go vet ./...`、`pnpm build`、`git diff --check` 通过。
- Nginx 1.24 官方镜像加载仓库配置并执行 `nginx -t` 通过；生产部署仍不依赖 Docker。
- systemd 255 能解析全部 Unit；Windows 挂载权限和 `/opt/echonote` 尚未安装产生的环境警告，按 Runbook 在真实 Staging 主机复验。
- `smoke.sh` 通过 `bash -n`，外置 Theme 初始化脚本通过 Node 语法检查，生产 `index.html` 不含内联 Script。
