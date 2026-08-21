# EchoNote 后端

Go 模块化单体，当前完成 Phase 1–9：基础设施、业务垂直切片，以及数据库 Session 身份与请求安全。

## 本地启动

要求 Go 1.25+、PostgreSQL 13+、`pg_trgm`、pgvector 0.8+、FFmpeg 与 FFprobe。Migration 会执行 `CREATE EXTENSION`，但 PostgreSQL 服务端必须先安装 pgvector 扩展文件。优先使用已有 PostgreSQL，并为 EchoNote 创建独立数据库。没有本地数据库时，仓库根目录提供可选 Compose：

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
go run ./cmd/api
go run ./cmd/worker
```

API 和 Worker 会在连接 PostgreSQL 时自动检查并补齐 Schema，不需要单独执行 Migration 命令；首次初始化使用的数据库账号需要有目标 Schema 的建表权限。两个服务是独立进程，内置数据库锁会避免并发初始化。Worker 处理 `resolve_episode`、转录状态机、Search 索引、Embedding 与按需 AI Artifact，同时续租并回收超时任务。`ECHONOTE_USER_ID` 只为 development / test 保留；staging / production 必须使用真实 Session，否则配置校验拒绝启动。

未配置 ASR / Object Storage 时，已有 Import、Library、Notes 与 Transcript 读取仍可运行；新建和重试转录返回 503，Worker 只注册已有业务 Job。启用转录至少需要：

```text
ASR_PROVIDER=aliyun
ASR_API_KEY=...
STORAGE_PROVIDER=aliyun_oss
STORAGE_REGION=cn-beijing
STORAGE_BUCKET=...
STORAGE_ACCESS_KEY=...
STORAGE_SECRET_KEY=...
```

完整配置和可选的 HTTPS Endpoint 见仓库根目录 `.env.example`。真实密钥不得提交。

未配置 Embedding 时，Notes/Transcript 关键词搜索和索引重建仍可运行，响应 `mode=keyword`。启用语义与 Hybrid Search：

```text
EMBEDDING_PROVIDER=aliyun
EMBEDDING_API_KEY=...
EMBEDDING_ENDPOINT=https://dashscope.aliyuncs.com
```

生产环境建议把 Endpoint 配成所属地域的百炼 Workspace 专属 HTTPS 地址。模型固定为 `text-embedding-v4`，向量维度固定为 1024，与 Migration 列类型一致。

未配置 LLM 时，已保存的 Artifact / Conversation 仍可读取，新建 Artifact 和发送消息返回 503。启用阿里云百炼 Qwen：

```text
LLM_PROVIDER=aliyun
LLM_API_KEY=...
LLM_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1
LLM_MODEL=qwen-plus
```

Artifact 只在显式请求时生成，付费 Job 不自动重试；相同输入会直接复用缓存。

## 健康检查

```text
GET /healthz  进程存活，不访问数据库
GET /readyz   PostgreSQL 就绪；不可用时返回 503
```

OpenAPI 源文件为 `openapi/openapi.yaml`。

## 身份与 Session

```text
POST /api/v1/auth/login   用户名和密码登录，设置 HttpOnly Session Cookie
POST /api/v1/auth/logout  撤销当前 Session 并清除 Cookie
GET  /api/v1/me           当前用户与 Session 过期时间
```

除登录、`/healthz` 和 `/readyz` 外，所有请求都要求有效 Session。staging / production 必须配置同域 HTTPS `PUBLIC_ORIGIN`；所有修改请求会拒绝缺失或不匹配的 `Origin`。认证 API 响应统一为 `Cache-Control: no-store`。

数据库只保存随机 Session Token 的 SHA-256 摘要。密码使用 bcrypt，先在目标主机确定参数：

```bash
go run ./cmd/admin benchmark-password
```

管理员创建、认领旧数据用户和重置密码时，密码只能从交互式终端或标准输入读取：

```bash
go run ./cmd/admin create <username>
go run ./cmd/admin claim <existing-user-id> <username>
go run ./cmd/admin reset-password <username>
```

重置密码会在同一事务撤销该用户全部 Session。没有公共注册、找回密码或 OAuth。

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

## Library API

```text
GET    /api/v1/episodes?limit=50&offset=0  最近导入列表
GET    /api/v1/episodes/{episode_id}       Episode、Podcast、Sources 与独立状态
DELETE /api/v1/episodes/{episode_id}       永久删除 Episode
```

所有查询和删除都按请求 Session 的 `user_id` 隔离。删除会级联清理 Source 与去重身份键，并在没有其他 Episode 时清理 Podcast；Import 与 Job 历史保留。

Library 列表的 `note_count` 只统计未删除 Note。删除仍在解析的 Capture Episode 时，关联 Import Job 会被取消，避免 Worker 之后重新创建该 Episode。

## Capture 与 Notes API

```text
POST   /api/v1/captures                    按 episode_id 或 episode_url 快速记录
GET    /api/v1/episodes/{episode_id}/notes 查询未删除 Note
POST   /api/v1/episodes/{episode_id}/notes 为已导入 Episode 新建 Note
PATCH  /api/v1/notes/{note_id}             编辑 Note
DELETE /api/v1/notes/{note_id}             幂等软删除 Note
```

PWA 必须为每条离线 Note 生成稳定 UUID `client_note_id`。首次创建返回 201；相同用户重复提交同一 ID 返回已保存 Note 和 200，不会重复写入。URL Capture 会原子创建 Pending Episode、Note 与 Import Job，并返回 `import_id` 供轮询。

## Transcription、Transcript 与 Speaker API

```text
POST /api/v1/episodes/{episode_id}/transcriptions       新建版本化转录
GET  /api/v1/transcriptions/{run_id}                    查询状态
GET  /api/v1/transcriptions/{run_id}/events             可恢复 SSE 事件
POST /api/v1/transcriptions/{run_id}/retry              从失败位置重试
POST /api/v1/transcriptions/{run_id}/cancel             取消
GET  /api/v1/episodes/{episode_id}/transcript           当前 Transcript Version
GET  /api/v1/transcripts/{transcript_id}/segments       分页读取 Segment
PATCH /api/v1/transcripts/{transcript_id}/speakers/{id} 重命名 / 更新角色
POST /api/v1/transcripts/{transcript_id}/speakers/merge 合并 Speaker
```

`economy` 使用 Paraformer-v2，`quality` 使用 Fun-ASR。音频统一转换为 16 kHz 单声道 FLAC，长音频按 90 分钟 Core Window 与 5 分钟左右重叠切片；算法和恢复语义见 `docs/architecture/transcription.md`。

## Search API

```text
GET  /api/v1/search?q=融资&scope=library&limit=20
GET  /api/v1/search?q=融资&scope=episode&episode_id={episode_id}
POST /api/v1/search/reindex
```

搜索同时覆盖当前用户未删除的 Notes、当前 active Transcript，以及最新 ready AI Artifact。关键词候选使用精确子串优先和 `pg_trgm` 模糊匹配；配置 Embedding 后，再与 pgvector 余弦候选通过 RRF 融合。结果包含 Episode、Podcast、Speaker、时间、摘要与排序分数。

重建请求体：

```json
{"scope":"episode","episode_id":"..."}
```

也可使用 `{"scope":"library"}` 重建整个 Library。Notes、Transcript Version 和 Speaker 变更会在原业务事务内自动创建 `build_keyword_index` Job。

## AI API

```text
GET  /api/v1/episodes/{episode_id}/ai/artifacts
POST /api/v1/episodes/{episode_id}/ai/artifacts
POST /api/v1/conversations
GET  /api/v1/conversations/{conversation_id}
POST /api/v1/conversations/{conversation_id}/messages  SSE
```

Artifact 一次返回一句话总结、核心观点、人物观点、值得回顾和笔记关联。Transcript、Speaker 或 Note 改变后旧结果保留但标记为 `stale`。Conversation 当前只支持 Episode Scope；每条消息需要稳定 UUID `client_message_id`，完成结果可幂等回放。Citation 只允许指向本次检索得到的真实 Segment / Note。

## Export API

```text
POST /api/v1/episodes/{episode_id}/exports
```

支持 `notes_only`、`organized_note`、`selected_transcript` 与 `full_transcript`。响应同时包含适合 Clipboard / iOS Share Sheet 的 `text`、可保存为文件的 `markdown` 和安全 `suggested_filename`。后端不直接写 Apple Notes；PWA 调用系统 Share Sheet 后由用户选择目标 App。

Export 是无状态同步操作，不创建数据库记录、Job 或对象存储文件。只读取 ready AI Artifact 和 active Transcript，并在同一个只读一致性快照中生成；任一输出超过 4 MiB 会明确返回 413，不静默截断。

## Schema 与代码生成

API 和 Worker 启动时会自动应用内嵌的 Schema 更新。以下命令仅用于检查版本或受控回滚，不是启动步骤：

```bash
go run ./cmd/migrate version
go run ./cmd/migrate down 1
go generate ./...
```

`down` 必须显式指定正整数步数，避免误回滚全部数据。生成器已固定版本，生成的 OpenAPI/SQL 代码提交到仓库，运行 API 和 Worker 不需要安装生成工具。

## 测试

```bash
go test ./...
go vet ./...
```

PostgreSQL 集成测试默认跳过。显式提供隔离的测试数据库后会执行 Migration，并验证 Session、CSRF、Job 生命周期、跨来源去重、Library 分页、Notes HTTP 生命周期、并发离线幂等、3 小时转录、Speaker ID 对调、单 Chunk 恢复、Transcript Version、Search、AI Artifact 缓存/失效、SSE Conversation、Citation 白名单、四种 Export Mode、用户隔离与删除级联：

```text
ECHONOTE_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_test?sslmode=disable
```

自动初始化集成测试使用可清空的独立数据库：

```text
ECHONOTE_SCHEMA_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/echonote_schema_test?sslmode=disable
```

测试使用随机用户 ID，并只清理自己创建的业务数据与 Job。

## 当前目录

```text
cmd/api/                 API 入口
cmd/worker/              Worker 入口
cmd/migrate/             Migration CLI
cmd/admin/               用户创建、认领、密码重置与 bcrypt 基准
internal/auth/           用户名规范化、密码哈希与 Session Token
internal/config/         环境配置
internal/database/       pgx 连接、migration 与 sqlc 生成代码
internal/http/           Chi 路由、OpenAPI handler、中间件
internal/domain/         Podcast、Transcription、Search、AI 与 Export 领域规则
internal/provider/       Podcast、音频、阿里云 ASR/Embedding/LLM、OSS 与安全 HTTP Provider
internal/repository/     业务持久化与 PostgreSQL Job Queue
internal/service/        Import、Transcription、Search、AI 与 Export 编排
internal/worker/         Job 消费、续租、分类重试与崩溃隔离
migrations/              版本化 SQL
openapi/                 HTTP 契约
```

实施记录见 `docs/architecture/phase-1-foundation.md` 至 `docs/architecture/phase-8-export.md`；固定转录算法见 `docs/architecture/transcription.md`。
