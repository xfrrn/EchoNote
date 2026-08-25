# EchoNote 内部服务

该 Go 模块只实现 EchoNote MCP 所需的本机数据与任务能力，不是公共 Web API。

## 启动

要求 Go 1.25+、PostgreSQL、pgvector、`pg_trgm`、FFmpeg 与 FFprobe。

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable'
$env:ECHONOTE_OWNER_ID = '00000000-0000-4000-8000-000000000001'
go run ./cmd/api
```

`ECHONOTE_OWNER_ID` 是 MCP 服务的数据所有者，不是登录用户。所有业务查询仍用该 ID 隔离数据。API 默认监听 `127.0.0.1:8080`，配置为非回环地址会拒绝启动。

development/test 会自动应用 Migration，并在 API 进程内启动 Worker；staging/production 使用独立 `cmd/worker`。Migration 9 删除旧网页认证的 Session 表、用户名与密码字段。

## 可选 Provider

未配置 ASR/对象存储时，导入、笔记与已有 Transcript 读取仍可运行；新转写不可用。

```text
ASR_PROVIDER=aliyun
ASR_API_KEY=...
ASR_STANDARD_MODEL=paraformer-v2
ASR_QUALITY_MODEL=fun-asr
STORAGE_PROVIDER=aliyun_oss
STORAGE_REGION=cn-beijing
STORAGE_BUCKET=...
STORAGE_ACCESS_KEY=...
STORAGE_SECRET_KEY=...
```

未配置 Embedding 时使用关键词搜索：

```text
EMBEDDING_PROVIDER=aliyun
EMBEDDING_API_KEY=...
```

未配置 LLM 时，已有 Artifact/Conversation 可读，但不能生成新内容：

```text
LLM_PROVIDER=aliyun
LLM_API_KEY=...
LLM_MODEL=qwen-plus
```

完整配置见仓库根目录 `.env.example`。

## 运维命令

```powershell
go run ./cmd/migrate version
go run ./cmd/migrate down 1
go run ./cmd/admin retry-cleanup <job-id>
go run ./cmd/maintenance retention --dry-run
go generate ./...
```

`down` 必须显式指定正整数步数。生产数据库升级到 Migration 9 前应备份；认证凭据和 Session 被删除后无法由 Down Migration 恢复。

## 验证

```powershell
go test ./...
go vet ./...
```

PostgreSQL 集成测试需显式提供一次性测试数据库：

```text
ECHONOTE_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_test?sslmode=disable
ECHONOTE_SCHEMA_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_schema_test?sslmode=disable
ECHONOTE_MIGRATION_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_migration_test?sslmode=disable
```

HTTP 契约源为 `openapi/openapi.yaml`，只供 `apps/mcp` 内部调用。转录切片与恢复算法见 `docs/architecture/transcription.md`。
