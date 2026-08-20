# Phase 3：Library 实施记录

更新时间：2026-08-20

## 范围

本阶段只完成当前开发顺序中的 Library：

- Episode 最近导入列表
- Episode 详情
- Podcast 元数据
- Episode Source 列表
- resolve / transcription / AI 独立状态
- Episode 删除

没有提前实现 Phase 4 Notes、转录、Search、AI、Export，也没有把 Web Demo 从 Mock Data 切换到后端。

## API

```text
GET    /api/v1/episodes?limit=50&offset=0
GET    /api/v1/episodes/{episode_id}
DELETE /api/v1/episodes/{episode_id}
```

### 列表

按 `episodes.created_at DESC, id DESC` 返回最近导入，响应包含：

- Episode 标题、发布时间、时长与封面
- 可缺省的 Podcast
- `resolve_status`
- `transcription_status`
- `ai_status`
- Source 数量
- 未删除 Note 数量（Phase 4 补充）
- `total / limit / offset`

`limit` 默认 50，允许 1–100；`offset` 必须为非负 32 位整数。当前产品是个人资料库，offset 分页足够简单；只有实际出现深分页性能问题时才改为 cursor，不预建两套分页协议。

### 详情

详情额外返回 Episode 描述与 Sources。每条 Source 包含：

```text
source_type
external_id
source_url
canonical_url
rss_guid
created_at
```

`audio_url` 仍保存在数据库供后续 Transcription Worker 使用，但不通过 Library API 暴露。EchoNote 不是播放器，前端查询资料库不需要音频直链，也不应泄露可能带签名参数的地址。

直接音频导入没有虚构 Podcast，因此响应中的 `podcast` 可以不存在。Episode 封面为空时，API 回退到 Podcast 封面。

### 状态

API 不生成单一聚合状态，直接返回数据库中的三个独立状态：

```text
resolve_status        pending | completed | failed
transcription_status  waiting | queued | running | completed | failed
ai_status             waiting | queued | running | completed | failed
```

这与 Web Demo 当前的单一 `EpisodeStatus` 不同。前端正式接入 API 时必须按页面用途映射这些状态，不能把 Demo 类型反向写入后端模型。

## 查询与用户隔离

所有列表、详情、Source 与删除 SQL 都包含 `user_id`。Podcast 使用 `(podcast.id, podcast.user_id)` 与 Episode 连接，Source 使用 `(episode_id, user_id)` 查询；仅知道其他用户的 Episode UUID 仍会得到 404。

列表使用一次有界查询与一次 `count(*)`，Source 数量由已有 `episode_sources_episode_idx` 支持。当前没有为个人规模引入缓存或独立读模型。

## 删除语义

`DELETE /episodes/{id}` 当前是明确的永久删除：

```text
按 user_id 删除 Episode
→ FK 级联删除 episode_sources
→ FK 级联删除 episode_identity_keys
→ imports.episode_id 置空，保留导入与 Job 历史
→ 没有其他 Episode 时删除孤立 Podcast
```

Episode 与孤立 Podcast 的处理在同一事务内。删除后重新导入同一期会创建新的 Episode。当前没有 Archive 产品入口，因此没有增加 `deleted_at` 或同时维护软删除与硬删除两套语义；如果产品以后明确需要“最近删除/恢复”，再通过 migration 增加归档字段和恢复 API。

保留 Import/Job 历史是为了任务审计；成功 Import 在 Episode 被删除后会保留 `succeeded`，但不再返回 `episode_id`。

Phase 4 增加了 Pending Capture Episode：删除这类 Episode 时会先取消仍为 `queued/running` 的关联 Import Job，再执行上述级联，防止 Worker 在删除后重新创建 Episode。Import 与 Job 记录仍保留用于审计。

## Migration 决策

Phase 2 Migration v2 已包含本阶段需要的 `podcasts`、`episodes`、`episode_sources`、关系、状态约束和删除行为。本阶段只有读 API 与已有关系上的事务删除，不需要修改 schema，因此没有创建空的 Migration v3。

下一个真实 schema 变化应由 Phase 4 Notes 创建 v3。空 migration 只会增加部署版本噪音，无法表达数据变更。

## 设计调整记录

### 1. Podcast 与 Source 嵌入 Episode API

整体方案把 Library 的职责写为 Podcast、Episode 与 Source，同时核心 API 的完成标准只列出 Episode 列表、详情和删除。本阶段在 Episode 响应中提供 Podcast，并在详情中提供 Sources；没有增加当前 UI 和完成标准都不使用的独立 Podcast 列表端点。

影响：数据模型没有减少，后续若增加按 Podcast 浏览的 UI，可直接在现有表上增加 `GET /podcasts`，无需改 Episode 契约。

### 2. Notes 数量延后

整体方案提到列表展示笔记数量，但当前开发顺序把 Notes 放在 Phase 4，Phase 3 当时没有 `notes` 表，因此没有返回伪造的 `note_count: 0`。Phase 4 已创建真实 Notes，并同步为列表增加只统计未删除记录的 `note_count`。

### 3. Web Demo 保持 Mock

当前任务是后端 Phase 3。Web 的 `EpisodeStatus`、Mock Episode 与导入动画没有静默改为半真实状态，避免在 Notes/Transcript API 尚不存在时形成混合数据源。

## 验证

```bash
go generate ./...
go test ./...
go vet ./...
pnpm build
```

设置 `ECHONOTE_TEST_DATABASE_URL` 后，PostgreSQL 集成测试验证：

- 最近导入排序、limit/offset 与 total
- 直接音频 Episode 的 Podcast 为空
- Podcast Episode 的详情与 Source
- 三套状态原样返回
- 其他用户无法列表、读取或删除
- 删除级联清理 Source 与身份键
- 最后一个 Episode 删除后清理 Podcast
- Import 历史保留且 `episode_id` 为空

2026-08-20 已在独立的本地 `echonote` 数据库（PostgreSQL 18，无 Docker）完成实测。另通过真实 Apple 导入执行完整 API → Worker → Library 流程：列表与详情返回正确 Podcast、Source 和状态，DELETE 返回 204，随后详情返回 404。端到端临时 Import/Job 已按核验后的精确 ID 删除。

## 下一阶段入口

Phase 4 已完成 Capture、Notes、离线同步幂等与真实 `note_count`，见 [`phase-4-notes.md`](phase-4-notes.md)。下一阶段按开发顺序进入 Transcription。
