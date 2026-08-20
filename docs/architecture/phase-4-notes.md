# Phase 4：Notes 实施记录

更新时间：2026-08-20

## 范围

本阶段完成：

- 已有 Episode 的 Note 创建与查询
- Capture 快速记录
- 未导入 URL 的“先记录、后解析”
- Note 编辑与软删除
- 弱网重试幂等
- Library 的真实 `note_count`

没有提前实现 Transcription、Search、AI、Export，也没有把 Web Demo 从 Mock Data 切换到后端。

## Migration v3

Migration `000003_notes` 新增 `notes`：

```text
id
user_id
episode_id
client_note_id
content
created_at
updated_at
deleted_at
```

约束与索引：

```text
UNIQUE (user_id, client_note_id)
episode_id → episodes.id ON DELETE CASCADE
(episode_id, created_at DESC, id DESC) WHERE deleted_at IS NULL
```

`created_at` 使用 PWA 提交的实际记录时间，不是播客播放时间；`updated_at` 与 `deleted_at` 由服务端维护。

## API

```text
POST   /api/v1/captures
GET    /api/v1/episodes/{episode_id}/notes
POST   /api/v1/episodes/{episode_id}/notes
PATCH  /api/v1/notes/{note_id}
DELETE /api/v1/notes/{note_id}
```

### Capture

`POST /captures` 必须且只能提供一个目标：

```json
{
  "client_note_id": "38a3b021-4f61-4f88-a59d-4301fc45fa9b",
  "episode_id": "7d51e84a-eedf-4324-a928-3e01ee757efe",
  "content": "这里关于 FDE 的定义和以前理解的不一样",
  "created_at": "2026-08-20T19:32:00+08:00"
}
```

或：

```json
{
  "client_note_id": "38a3b021-4f61-4f88-a59d-4301fc45fa9b",
  "episode_url": "https://podcasts.apple.com/...",
  "content": "这个观点后面需要重新梳理",
  "created_at": "2026-08-20T19:32:00+08:00"
}
```

已有 Episode 模式只创建 Note。URL 模式在同一 PostgreSQL 事务中：

```text
锁定 (user_id, client_note_id)
→ 创建 resolve_status=pending 的 Episode
→ 创建 Import 与 resolve_episode Job
→ 创建 Note
```

响应同时返回 Note 与 `import_id`，PWA 使用现有 Import API 轮询解析结果。

### Pending Episode 解析与去重

Worker 解析 URL 后：

- 没有命中已有身份键：直接补全原 Pending Episode，Episode ID 不变。
- 命中已有 Episode：把 Pending Episode 下的 Note 移到已有 Episode，删除空的 Pending Episode，并让 Import 指向最终 Episode。

这保留了 Phase 2 的跨来源去重，不会因为“先记后导入”产生永久重复单集。

如果用户在解析前永久删除 Pending Episode，Library 删除事务会取消关联的 `queued/running` Import Job。Worker 即使已经领取任务也不能在之后恢复已删除 Episode；Import/Job 审计记录保留。

## 离线幂等

PWA 为每条本地 Note 生成稳定 UUID `client_note_id`，并在网络恢复后重试同一请求。服务端同时使用：

```text
PostgreSQL advisory transaction lock
+
UNIQUE (user_id, client_note_id)
```

因此并发重放也只创建一条记录。首次创建返回 201；重复请求返回第一次保存的 Note 和 200。若该 Note 后来已软删除，重放不会恢复它，响应中的 `deleted_at` 会标识 tombstone。

`client_note_id` 是一次创建操作的永久身份，客户端不得把同一 ID 用于另一条内容或另一个 Episode；后续 payload 不会覆盖第一次保存的值。

当前离线范围是单条 Note 的本地排队与安全重试，符合原设计中 `client_note_id` 的定义。没有增加未被当前产品需要的批量同步或多设备增量游标；出现真实的多设备同步需求时，再基于 `updated_at/deleted_at` 增加 delta API。

## Note 生命周期

- 列表只返回 `deleted_at IS NULL` 的 Note，按客户端 `created_at DESC, id DESC` 排序。
- PATCH 只允许修改未删除 Note 的 `content`。
- DELETE 设置 `deleted_at`，重复 DELETE 仍返回 204。
- 永久删除 Episode 会通过 FK 级联永久删除其 Notes。
- 所有 SQL 均带 `user_id`；其他用户的 Episode 或 Note 统一表现为 404。

Library 列表新增 `note_count`，只统计未删除 Note，不返回伪造数量。

## 设计调整记录

### 1. 保留两个创建入口

`POST /captures` 服务于全局快速记录并支持 URL；`POST /episodes/{id}/notes` 服务于 Episode 详情页。两者复用同一幂等 Repository，不维护两套写入语义。

### 2. Web Demo 暂不接后端

当前 Phase 是后端垂直切片。Web 仍使用 Zustand 持久化 Mock Note；等 Library、Notes 与后续 Transcript 的前端接入边界一起确认后再切换，避免混合真实与 Mock Episode ID。

## 验证

```bash
go generate ./...
ECHONOTE_TEST_DATABASE_URL=postgres://... go test ./... -count=1
go vet ./...
pnpm build
```

PostgreSQL 自动化测试覆盖：

- 同一 `client_note_id` 并发创建只落一条 Note
- URL Capture 的 Episode、Import、Job、Note 原子创建
- Pending Episode 原地补全与命中已有 Episode 后的 Note 迁移
- 删除 Pending Episode 后取消 Job，解析不会复活数据
- Note 查询、编辑、重复软删除与真实 `note_count`
- 用户隔离与 Episode 删除级联
- HTTP 的 201/200/204、请求校验及 OpenAPI DTO

2026-08-20 已在独立本地 `echonote` 数据库（PostgreSQL 18，无 Docker）完成 API + Worker 实测：Direct Audio URL Capture 解析成功，Note 的查询、编辑、幂等删除和 Episode 删除状态均符合契约。临时 Import/Job 已按精确 UUID 永久删除，数据库业务表为空。

## 下一阶段入口

Phase 5 按转录设计实现音频处理、Chunk、阿里云 ASR、Speaker Alignment 与 Transcript Version，不在 Notes Worker 中提前耦合转录逻辑。
