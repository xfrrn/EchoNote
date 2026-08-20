# Phase 6：Search 实施记录

更新时间：2026-08-20

## 范围

本阶段按垂直切片完成：

- Notes 与 active Transcript 的统一 Search Document
- 精确子串优先与 `pg_trgm` 模糊关键词搜索
- 阿里云 `text-embedding-v4` Embedding Provider
- pgvector 余弦检索与 RRF Hybrid 排序
- Library / Episode 两种搜索范围
- 自动增量重建与显式全量重建
- Episode、Speaker、时间和摘要响应

没有提前实现 AI Artifact 生成、Conversation、SSE Chat 或 Export。`ai_artifact` 文档类型只作为 Phase 7 的兼容入口保留；当前 Web Demo 仍使用 Mock Data，后端契约以 OpenAPI 0.6.0 为准。

## 实现前分析

当前代码已经具备 Notes、active Transcript Version、Speaker 修正、PostgreSQL Job Queue 与原子业务事务。Search 最小可靠路径因此是复用这些能力：

```text
业务写入
→ 同一事务 enqueue build_keyword_index(Episode)
→ 重建可派生 Search Document / Chunk
→ 仅缺失或模型不匹配时 enqueue generate_embeddings(Document)
→ GET /search 查询 PostgreSQL
```

没有引入 Elasticsearch、Redis、Kafka、独立搜索服务或新的基础设施。

## Migration v5

`000005_search` 安装并使用：

```text
pg_trgm
vector
search_documents
search_chunks
```

`search_documents` 以 `(user_id, document_type, source_id)` 唯一，保存源内容、结构化 `content_hash` 和 metadata。`search_chunks` 保存文本、Speaker、时间、1024 维向量和 `embedding_model`。

索引包括用户 / Episode / 文档类型及 Document → Chunk 的 B-tree。关键词候选使用精确子串和低阈值 `word_similarity`；该函数条件不会稳定利用 GIN，因此本阶段不保留无效写放大的 trigram 索引。达到真实慢查询阈值后，应把精确与模糊候选拆成可索引 operator 查询，再增加 GIN/GiST。

向量检索本阶段使用 pgvector 精确 cosine scan。没有建立 HNSW：全局近似候选在 `user_id/episode_id` 过滤后可能降低召回，而个人 Library 的当前数据量不值得用正确性换性能；达到真实慢查询阈值后再增加带过滤策略的近似索引。

Migration 会为升级前已有 Episode 创建 `build_keyword_index` Job，因此存量 Notes/Transcript 不需要手工搬运。Down Migration 删除本 Phase 的 Search Job 与业务表，但不删除可能由数据库管理员预先安装并由其他对象使用的扩展。

## Search Document 与一致性

当前映射：

```text
未删除 Note              → 一个 note Document + 一个 Chunk
active Transcript Version → 一个 transcript Document + 多个 Chunk
旧 Transcript Version     → 不进入 Search
```

Episode 删除依靠外键级联删除 Search 数据，并先取消对应 Search Job。以下写入会在原业务事务内创建重建 Job：

- Note 创建、修改、软删除
- URL Capture 解析后发生 Episode 合并
- 新 Transcript Version 激活
- Speaker 重命名或合并

重建先锁定所属 Episode，再从业务真相表重新派生索引；重复 Job 不会累积重复 Document。`content_hash` 包含文档类型、source、文本、Speaker、时间和 Chunk 结构。只有 Hash 变化或模型变化才生成新向量。

Speaker 名称不复制到 Search 表，查询时实时关联 `transcript_speakers`。因此纯重命名的重建会安全 no-op，也不会产生 Embedding 费用；Speaker 合并会改变 Chunk 的 Speaker/分组并触发真正重建。

## Transcript Chunk 调整

整体方案要求约 300–600 个中文字符并少量重叠，但还要求每条结果准确返回 Speaker 和时间。对话中 Speaker 可能在 300 字前切换，强行跨 Speaker 合并会使 `speaker_id` 不再真实。

本阶段明确为：

- 只合并同一 Speaker 的连续 Segment。
- 约 600 字时切分。
- 仅复用总计不超过约 80 字的完整尾部 Segment，避免伪造字符级时间。
- Speaker 切换时立即结束当前 Chunk；短块允许低于 300 字。
- 单个 ASR Segment 若本身超过 600 字，不伪造时间切片，保留为一个超长 Chunk。

影响是快速对话会产生较短向量，但 Speaker、时间和 Citation 边界保持可信。该规则由领域测试锁定。

## Embedding Provider

业务层依赖：

```go
Embed(ctx, texts, inputType)
Model()
Dimensions()
```

当前 Provider 使用阿里云 DashScope 同步 Embedding API：

```text
model      = text-embedding-v4
dimension  = 1024
text_type  = query | document
batch size = 1..10
```

Provider 校验 HTTPS Endpoint、禁止 URL 凭据、限制响应体、恢复 `text_index` 顺序，并拒绝数量、维度、NaN、Inf 或零向量错误。API Key 不写入数据库或日志。生产 HTTP Client 继续使用项目现有的公网地址校验。

实现依据：

- [阿里云文本向量同步 API](https://www.alibabacloud.com/help/en/model-studio/text-embedding-synchronous-api)
- [pgvector 官方文档](https://github.com/pgvector/pgvector)
- [PostgreSQL pg_trgm 文档](https://www.postgresql.org/docs/18/pgtrgm.html)

## Job 与费用边界

```text
build_keyword_index  → 可重试 3 次，不调用付费 Provider
generate_embeddings  → 每次最多 10 个 Chunk，成功一批立即持久化
```

Embedding Job 的 `max_attempts=1`，网络歧义或 Provider 失败不会由 Queue 自动重放付费请求。用户可通过 `POST /api/v1/search/reindex` 明确重试；已成功批次不会再次生成，只有仍缺失的 Chunk 会调用 Provider。

未配置 Provider 时：

- Worker 仍处理 `build_keyword_index`。
- 不创建新的 Embedding Job。
- API 返回关键词结果与 `mode=keyword`。

Provider 临时失败时，单次搜索降级为关键词结果并记录不含密钥的 Warning；数据库错误不会被静默降级。

## 检索与排序

关键词候选：

```text
精确、不区分大小写的子串
→ word_similarity(pg_trgm) 模糊候选
```

语义候选使用 pgvector cosine distance。两组候选各取请求 limit 的三倍，然后使用 RRF（`k=60`）按 Chunk ID 融合；同一 Chunk 同时命中两路时自然获得更高分。摘要以查询命中位置为中心截取最多 180 个 Unicode 字符。

所有 SQL 都包含 `user_id`，Episode Scope 还会先验证 Episode 所有权。Search 结果不会跨用户泄漏。

## API

OpenAPI 0.6.0 新增：

```text
GET  /api/v1/search
POST /api/v1/search/reindex
```

GET 参数：

```text
q           必填，2–500 字符
scope       library | episode，默认 library
episode_id  episode scope 必填
limit       1–50，默认 20
```

响应 `mode` 为 `keyword` 或 `hybrid`，每项包含 Document 类型、source、Episode、Podcast、可选 Speaker/时间、snippet 和 RRF score。

## 本地 PostgreSQL 验收调整

用户要求复用现有 `localhost:5432`，同时使用独立 `echonote` 数据库且不使用 Docker。该 PostgreSQL 18.3 原先有 `pg_trgm`，但未安装 pgvector；系统安装目录又拒绝普通用户写入。

本机验收采用官方 pgvector v0.8.6 源码编译产物，放在应用专用目录 `C:\ProgramData\EchoNote\PostgreSQL\18`，并只对 `echonote` 设置 PostgreSQL 18 支持的 `extension_control_path` 与 `dynamic_library_path`。标准 Migration 仍只执行 `CREATE EXTENSION vector`，没有硬编码本机路径，也没有修改或迁移 `autoup` 数据库。依据：[PostgreSQL 18 动态库与扩展路径](https://www.postgresql.org/docs/18/runtime-config-client.html#RUNTIME-CONFIG-CLIENT-OTHER)。

生产部署仍应由数据库管理员按目标 PostgreSQL 版本正常安装 pgvector，不应复制本机验收路径。

仓库的可选 Compose 同步改为 pgvector 官方 `0.8.6-pg17` 镜像；本地本阶段验收没有启动它。原 `postgres:17-alpine` 不包含 vector 扩展，会使 Migration v5 失败，因此该调整是保持已有可选启动方式可运行所必需的。

## 验收

本阶段自动验收覆盖：

- Migration v5、`pg_trgm 1.6` 与 `vector 0.8.6` 真实创建。
- 同一 Episode 的 Note + Transcript 构建与幂等重建。
- Transcript Chunk 的 Speaker 边界、长度目标和完整 Segment 重叠。
- 阿里云请求路径、鉴权、model、dimension、`text_type`、乱序响应恢复和错误向量拒绝。
- Notes 精确与拼写容错关键词命中；无关键词重合的查询通过向量命中 Transcript。
- Embedding Provider 查询失败时降级关键词结果，并保留可观测错误。
- Hybrid 结果返回 Episode、Speaker、时间和摘要。
- 跨用户搜索为空；Episode Scope 校验所有权。
- 未变化重建不新增 Embedding Job；Note 修改只重建对应内容。
- GET Search 与 POST Reindex 的真实 PostgreSQL HTTP 链路。
- Phase 1–5 全量 PostgreSQL 回归。

本地测试使用独立的 `postgres://postgres:<password>@localhost:5432/echonote?sslmode=disable`，没有使用 Docker，没有访问 `autoup`，也没有调用真实阿里云 Embedding，因此未产生云端费用。部署前仍需用目标百炼账号执行一条受控 smoke test。
