# Phase 1：后端项目基础实施记录

更新时间：2026-08-20

## 范围

本阶段只实现当前开发顺序中的 Phase 1：

- Config
- Logging
- PostgreSQL
- Migration
- OpenAPI
- Health Check
- PostgreSQL Job Queue
- 可独立启动的 API 与 Worker

没有提前实现 Auth、Import、Library、Notes、转录、Search、AI、Export 或任何外部 Provider。

## 阅读结论

实现前已核对现有 Web 页面、Mock、TypeScript 类型和全部 `docs/` 内容：

- Web 当前完全使用 Zustand 与本地 Mock，没有真实 HTTP 调用。
- `packages/contracts` 仍是占位；当前后端 HTTP 契约以 OpenAPI 为唯一来源。
- `docs/` 当前只有《EchoNote 后端整体实现方案 v1.0》一份实际设计文档；数据库、转录、AI 和 Search 设计均包含在其中，文档中提到的拆分文件尚不存在。
- 前端展示的单一 `EpisodeStatus` 是 Demo 类型；后端后续仍按设计保留独立的 resolve/transcription/AI 状态，不把 Demo 类型直接当数据库模型。

## 实现结构

```text
apps/server/
├── cmd/
│   ├── api/
│   ├── migrate/
│   └── worker/
├── internal/
│   ├── config/
│   ├── database/
│   │   ├── db/          # sqlc 生成
│   │   └── queries/
│   ├── http/            # OpenAPI 生成代码 + handler
│   ├── logging/
│   ├── repository/      # Job Queue
│   └── worker/
├── migrations/
└── openapi/
```

## 设计调整记录

### 1. 沿用当前仓库骨架

整体方案的第 10 节建议 `internal/modules` 与 `internal/platform`，但仓库已存在并说明 `domain/service/provider/repository/database/worker` 结构。

本阶段没有为目录命名重组空骨架，而是沿用现有 `config/http/database/repository/worker`。原因是 Phase 1 只有基础设施，没有业务模块；现在搬到另一套目录只会制造无功能收益的重写。

影响：后续进入 Import 垂直切片时，再根据第一个真实业务模块决定模块内部边界；不会要求一次性迁移整个后端。

### 2. 以当前 Phase 清单为边界

整体方案的“推荐开发顺序”把 Auth、Session 和 Object Store 也列入第一阶段；本次任务给出的 Phase 1 明确只包含 Config、Logging、PostgreSQL、Migration、OpenAPI、Health Check 和 Job Queue。

本阶段以当前任务清单为准，没有提前实现 Auth 或 Object Store。

影响：`jobs.user_id` 暂时允许为空且尚无外键。进入 Auth 阶段后创建 `users`，再用后续 migration 添加外键；业务 Job 必须在具备用户上下文时写入 `user_id`。

### 3. 健康端点不放在 `/api/v1`

业务 API 仍使用 `/api/v1`。进程探针采用根路径：

```text
GET /healthz
GET /readyz
```

`/healthz` 只确认进程存活；`/readyz` 在两秒超时内检查 PostgreSQL，并在不可用时返回 503。这些端点服务于部署探针，不属于版本化产品 API。

### 4. Job Queue 的 Phase 1 边界

Migration 创建：

- `jobs`
- `job_events`
- queued/running 的部分索引
- 状态、锁、attempt 与 completed 时间的一致性约束

领取任务使用单条 `UPDATE ... FROM (SELECT ... FOR UPDATE SKIP LOCKED)`，同一事务写入事件。Worker 支持：

- 按已注册 Job 类型领取
- 锁续租
- 超时租约回收
- 有限重试
- handler panic 隔离
- 成功、重试、失败事件
- 进程退出后由租约恢复未完成任务

Phase 1 没有业务 Job handler。Worker 仍可启动并维护租约；Phase 2 的 Import slice 会注册首个任务类型。

### 5. 代码生成方式

- 数据库访问：`pgx/v5 + sqlc`
- HTTP：`Chi + OpenAPI + oapi-codegen`
- Migration：`golang-migrate` 的 pgx/v5 driver
- Logging：标准库 `log/slog`

生成工具通过 `go generate ./...` 固定版本运行，不要求开发机全局安装，也不会进入服务运行时依赖。

## 运行与验证

基础检查：

```bash
go generate ./...
go test ./...
go vet ./...
```

显式提供 `ECHONOTE_TEST_DATABASE_URL` 时，集成测试会：

```text
应用 Migration
→ enqueue Job
→ SKIP LOCKED claim
→ complete
→ 验证 Job 状态与 3 条事件
→ 验证失败重试与 attempt 耗尽后的 failed 状态
→ 删除该测试 Job（事件级联删除）
```

数据库连接串和云服务密钥不会写入日志或仓库。

2026-08-20 已在独立的新建 `echonote` 数据库（PostgreSQL 18）完成实测：Migration version 为 1、`dirty=false`；Job Queue 生命周期集成测试通过；API 两个探针均返回 200；Worker 可连接、启动并优雅停止。既有数据库未执行 EchoNote Migration。Docker 未用于本次数据库验证。

## 下一阶段入口

Phase 2 只需要在本基础上新增 Import 垂直切片：Resolver Provider、Podcast/Episode 最小数据模型、Import API 和对应 Job handler。当前阶段没有为这些未来实现预建空接口或业务表。
