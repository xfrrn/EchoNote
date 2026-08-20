# EchoNote 后端

Go 模块化单体，当前完成 Phase 1 基础设施：API、Worker、PostgreSQL Migration、OpenAPI 健康检查和 PostgreSQL Job Queue。

## 本地启动

要求 Go 1.25+ 与 PostgreSQL（最低支持 PostgreSQL 13，需提供 `gen_random_uuid()`）。优先使用已有 PostgreSQL，并为 EchoNote 创建独立数据库。没有本地数据库时，仓库根目录提供可选 Compose：

```bash
docker compose up -d postgres
```

设置环境变量；完整清单见仓库根目录 `.env.example`：

```text
DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable
```

在 `apps/server` 下依次执行：

```bash
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/worker
```

API 和 Worker 是独立进程。Phase 1 尚无业务 Job handler，因此 Worker 会保持空闲，同时负责回收过期租约；后续 Phase 在启动时注册自己的 Job 类型。

## 健康检查

```text
GET /healthz  进程存活，不访问数据库
GET /readyz   PostgreSQL 就绪；不可用时返回 503
```

OpenAPI 源文件为 `openapi/openapi.yaml`。

## Migration 与代码生成

```bash
go run ./cmd/migrate version
go run ./cmd/migrate up
go run ./cmd/migrate down 1
go generate ./...
```

`down` 必须显式指定正整数步数，避免误回滚全部数据。生成器已固定版本，生成的 OpenAPI/SQL 代码提交到仓库，运行 API 和 Worker 不需要安装生成工具。

## 测试

```bash
go test ./...
go vet ./...
```

PostgreSQL 集成测试默认跳过。显式提供隔离的测试数据库后会执行 Migration，并验证 Job 的 enqueue、claim、complete、retry、attempt 耗尽失败与 events 生命周期：

```text
ECHONOTE_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_test?sslmode=disable
```

测试只清理自己创建的 Job。

## 当前目录

```text
cmd/api/                 API 入口
cmd/worker/              Worker 入口
cmd/migrate/             Migration CLI
internal/config/         环境配置
internal/database/       pgx 连接、migration 与 sqlc 生成代码
internal/http/           Chi 路由、OpenAPI handler、中间件
internal/repository/     PostgreSQL Job Queue
internal/worker/         Job 消费、续租、重试与崩溃隔离
migrations/              版本化 SQL
openapi/                 HTTP 契约
```

Phase 1 的结构取舍与设计差异见 `docs/architecture/phase-1-foundation.md`。
