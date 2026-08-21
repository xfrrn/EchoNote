# 全新主机部署 Runbook

本 Runbook 以 Ubuntu 24.04、systemd、Nginx 和独立托管 PostgreSQL 为例。生产过程不使用 Docker。命令中的域名、主机、Release ID 和账号必须替换为当前环境值。

## 1. 准备输入

- 已解析到主机的域名，例如 `notes.example.com`。
- 独立的 EchoNote PostgreSQL 数据库、服务端 CA、连接预算和每日备份。
- Production 专用的 OSS Bucket、ASR、Embedding、LLM 账号与限额；Staging 使用另一套资源。
- 一个已通过 CI 的 Git commit 和不可变 Release ID。

## 2. 初始化主机

```bash
sudo apt-get update
sudo apt-get install -y nginx certbot python3-certbot-nginx postgresql-client jq curl ffmpeg ca-certificates
sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin echonote-api
sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin echonote-worker
sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin echonote-migrate
sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin echonote-maintenance
sudo install -d -o root -g root -m 0755 /opt/echonote/releases
sudo install -d -o root -g root -m 0711 /etc/echonote
sudo install -d -o echonote-worker -g echonote-worker -m 0700 /var/lib/echonote/tmp
```

主机防火墙只允许 SSH、80 和 443 入站；8080 不得对外开放。API 的 `SERVER_HOST` 必须保持 `127.0.0.1`。

## 3. 准备 PostgreSQL

在数据库管理面创建 `echonote_migrate`、`echonote_api`、`echonote_worker` 和 `echonote_maintenance`。四个角色必须是 `NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS`；只有 Migration 使用的 `echonote_migrate` 持有目标数据库，其他进程从不使用 Owner 或 `postgres`。

创建由 `echonote_migrate` 持有的独立 `echonote` 数据库，确认目标 PostgreSQL 版本已安装匹配版本的 pgvector。先由托管数据库管理员执行 `deployments/postgres/extensions.sql`，再以 `echonote_migrate` 执行：

```bash
psql "$OWNER_DATABASE_URL" \
  -v database_name=echonote \
  -v migration_role=echonote_migrate \
  -v api_role=echonote_api \
  -v worker_role=echonote_worker \
  -v maintenance_role=echonote_maintenance \
  -f deployments/postgres/runtime-grants.sql
```

数据库管理员 URL 只在数据库管理终端使用，不复制到应用主机。确认 `pg_trgm`、`vector` 已启用，Runtime Role 没有数据库或 `public` Schema 的 `CREATE` 权限。若升级已有独立 EchoNote 数据库，先由管理员把应用表、序列和函数的 Owner 一次性转给 `echonote_migrate`；不得对共享数据库执行 `REASSIGN OWNED`。

模板连接上限为 API 10 + Worker 5 + Migration 2 + Maintenance 1 = 18，低于应用预算 20；数据库 `max_connections` 还必须为托管监控、备份和管理员保留容量。扩容 Worker 前先重算总和，不能只提高单个 URL。

## 4. 构建不可变 Release

在可信构建机从干净 commit 构建：

```bash
corepack enable
pnpm install --frozen-lockfile
pnpm generate:contracts
(cd apps/server && go generate ./...)
git diff --exit-code
pnpm build
(cd apps/server && go vet ./...)
(cd apps/server && ECHONOTE_TEST_DATABASE_URL="$TEST_DATABASE_URL" go test -p 1 ./... -count=1)

mkdir -p release/bin release/web release/ops
CGO_ENABLED=0 go -C apps/server build -trimpath -o ../../release/bin/echonote-api ./cmd/api
CGO_ENABLED=0 go -C apps/server build -trimpath -o ../../release/bin/echonote-worker ./cmd/worker
CGO_ENABLED=0 go -C apps/server build -trimpath -o ../../release/bin/echonote-migrate ./cmd/migrate
CGO_ENABLED=0 go -C apps/server build -trimpath -o ../../release/bin/echonote-admin ./cmd/admin
CGO_ENABLED=0 go -C apps/server build -trimpath -o ../../release/bin/echonote-maintenance ./cmd/maintenance
cp -a apps/web/dist/. release/web/
cp -a deployments/. release/ops/
```

将 `release/` 上传到 `/opt/echonote/releases/<release-id>`。Release 目录由 root 持有、只读，不覆盖已有 Release。

## 5. 安装配置

把 `deployments/env/*.example` 复制成 `/etc/echonote/{common,api,worker,migration,maintenance}.env`，填入密钥管理系统提供的值，然后：

```bash
sudo chown root:root /etc/echonote/*.env
sudo chmod 0600 /etc/echonote/*.env
sudo install -o root -g root -m 0644 db-ca.pem /etc/echonote/db-ca.pem
```

`/etc/echonote` 使用 `0711`，让非 root 服务只能穿越目录读取公开的数据库 CA；环境文件和私钥仍保持 `0600 root:root`，服务用户不能直接读取。

环境文件不能包含 `ECHONOTE_USER_ID`。数据库用户名和密码必须按 URL 规则编码，模板中的 `CHANGE_ME` 会被预检拒绝。不要在 shell 中 `source` 这些文件；URL 中的 `&` 也不是 shell 语法。密钥轮换只更新环境文件并重启对应服务，不重新构建二进制。

安装本 Release 自带的托管配置（把 `<release-id>` 替换为刚上传且已核对的目录）：

```bash
sudo install -o root -g root -m 0644 /opt/echonote/releases/<release-id>/ops/systemd/*.service /opt/echonote/releases/<release-id>/ops/systemd/*.timer /etc/systemd/system/
sudo install -o root -g root -m 0644 /opt/echonote/releases/<release-id>/ops/nginx/echonote-proxy.conf /etc/nginx/snippets/echonote-proxy.conf
sudo install -o root -g root -m 0644 /opt/echonote/releases/<release-id>/ops/nginx/echonote.conf /etc/nginx/conf.d/echonote.conf
sudo systemctl daemon-reload
```

替换 Nginx 文件中的域名和证书路径。首次签发证书可先用 Certbot standalone；之后启用 `certbot.timer` 并执行一次 `certbot renew --dry-run`。

## 6. 发布

1. 确认数据库备份任务刚刚成功，并记录备份 ID。
2. 确认 Release 目录完整，记录当前 `/opt/echonote/current` 指向作为回滚目标。
3. 原子切换 symlink；运行中的旧进程不受影响：

```bash
sudo ln -s "/opt/echonote/releases/<release-id>" /opt/echonote/current.next
sudo mv -Tf /opt/echonote/current.next /opt/echonote/current
```

4. 在改流前校验配置和托管文件：

```bash
sudo systemd-analyze verify /etc/systemd/system/echonote-*.service /etc/systemd/system/echonote-*.timer
sudo nginx -t
sudo systemd-run --quiet --wait --pipe --collect --uid=echonote-api \
  -p EnvironmentFile=/etc/echonote/common.env -p EnvironmentFile=/etc/echonote/api.env \
  /opt/echonote/current/bin/echonote-api --check-config
sudo systemd-run --quiet --wait --pipe --collect --uid=echonote-worker \
  -p EnvironmentFile=/etc/echonote/common.env -p EnvironmentFile=/etc/echonote/worker.env \
  /opt/echonote/current/bin/echonote-worker --check-config
```

5. 独立执行 Up Migration；失败则立即恢复旧 symlink，不重启应用：

```bash
sudo systemctl start echonote-migrate.service
sudo journalctl -u echonote-migrate.service -n 100 --no-pager
```

6. 以 `echonote_migrate` 再执行一次基础授权，然后应用 Worker 身份表隔离和 Maintenance 列级授权；核对 Runtime DML 权限。默认权限已保证后续 Migration 继承：

```bash
psql "$MIGRATION_DATABASE_URL" \
  -v database_name=echonote \
  -v migration_role=echonote_migrate \
  -v api_role=echonote_api \
  -v worker_role=echonote_worker \
  -v maintenance_role=echonote_maintenance \
  -f /opt/echonote/current/ops/postgres/runtime-grants.sql
psql "$MIGRATION_DATABASE_URL" \
  -v worker_role=echonote_worker \
  -f /opt/echonote/current/ops/postgres/runtime-table-grants.sql
psql "$MIGRATION_DATABASE_URL" \
  -v maintenance_role=echonote_maintenance \
  -f /opt/echonote/current/ops/postgres/maintenance-grants.sql
```
7. 启动并设为开机恢复：

```bash
sudo systemctl enable --now echonote-api.service echonote-worker.service nginx.service certbot.timer
sudo systemctl restart echonote-api.service echonote-worker.service
sudo systemctl reload nginx.service
```

8. 检查本机 readiness、服务状态和 JSON 日志：

```bash
curl --fail --silent http://127.0.0.1:8080/readyz
sudo systemctl --no-pager --full status echonote-api echonote-worker nginx
sudo journalctl -u echonote-api -u echonote-worker --since '-5 minutes' --no-pager
```

9. 运行 `/opt/echonote/current/ops/scripts/smoke.sh https://notes.example.com`，密码只从交互式终端读取。

## 7. 首次环境验收

- `systemd-analyze security echonote-api echonote-worker`，确认服务为独立非 root 用户。
- `sudo -u echonote-api test ! -r /etc/echonote/worker.env`，反向亦然；Worker DB Role 对 `users`/`sessions` 无任何权限、对 `notes` 只读。
- 从另一台主机确认 8080 不可达，80 只跳转 443。
- 使用真实 Run ID 让 SSE 持续连接并带 `Last-Event-ID` 重连。
- 在 Staging 运行任务时终止 Worker；等待 Lease 超时并重启，确认 Job 被回收且没有重复 Transcript。
- 重启 API、Worker 和主机，确认 Session、Job 和数据不丢失。
- 按 Phase 12 Runbook 完成 Provider、告警、备份恢复、桌面与 iOS PWA 验收后才能放量。
- 先按 Operations Runbook 审核留存 Dry Run 统计，再显式启用 `echonote-retention.timer`；部署流程不会自动启用删除。
