# EchoNote 后端

Go 模块化单体，当前完成 Phase 1 基础设施与 Phase 2 Import：Apple Podcasts、RSS、直接音频解析，异步 Episode 创建和跨来源去重。

## 本地启动

要求 Go 1.25+ 与 PostgreSQL（最低支持 PostgreSQL 13，需提供 `gen_random_uuid()`）。优先使用已有 PostgreSQL，并为 EchoNote 创建独立数据库。没有本地数据库时，仓库根目录提供可选 Compose：

```bash
docker compose up -d postgres
```

设置环境变量；完整清单见仓库根目录 `.env.example`：

```text
DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable
ECHONOTE_USER_ID=00000000-0000-4000-8000-000000000001
```

在 `apps/server` 下依次执行：

```bash
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/worker
```

API 和 Worker 是独立进程。Worker 当前处理 `resolve_episode`，同时续租并回收超时任务。Auth 尚未进入当前 Phase，`ECHONOTE_USER_ID` 暂时提供单用户数据边界。

## 健康检查

```text
GET /healthz  进程存活，不访问数据库
GET /readyz   PostgreSQL 就绪；不可用时返回 503
```

OpenAPI 源文件为 `openapi/openapi.yaml`。

## 导入 API

```text
POST /api/v1/imports              创建异步导入，返回 202
GET  /api/v1/imports/{import_id}  查询状态与 episode_id
```

请求示例：

```json
{"url":"https://podcasts.apple.com/.../id123?i=456"}
```

支持带 `i` 单集参数的 Apple Podcasts 链接、RSS Feed URL 与直接音频 URL。RSS Feed 默认导入其中发布时间最新的音频单集。

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

PostgreSQL 集成测试默认跳过。显式提供隔离的测试数据库后会执行 Migration，并验证 Job 生命周期以及不同来源导入同一期时的 Episode 去重：

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
internal/domain/         Resolver 领域契约
internal/provider/       Apple、RSS、直接音频 Provider 与安全 HTTP Client
internal/repository/     Import 持久化、Episode 去重、PostgreSQL Job Queue
internal/service/        Import Job 编排
internal/worker/         Job 消费、续租、分类重试与崩溃隔离
migrations/              版本化 SQL
openapi/                 HTTP 契约
```

实施记录见 `docs/architecture/phase-1-foundation.md` 与 `docs/architecture/phase-2-import.md`。
