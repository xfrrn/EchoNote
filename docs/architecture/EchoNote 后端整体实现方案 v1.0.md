# EchoNote 后端整体实现方案 v1.0

> 实施记录：当前任务把原推荐“第二阶段”拆成独立的 Import、Library、Notes Phase。Import 的实际边界、数据库并发去重与安全策略见 [`phase-2-import.md`](phase-2-import.md)；Library 的查询、状态与删除语义见 [`phase-3-library.md`](phase-3-library.md)；Capture、离线幂等与 Notes 生命周期见 [`phase-4-notes.md`](phase-4-notes.md)；完整转录、Speaker 对齐、版本与云端任务恢复见 [`phase-5-transcription.md`](phase-5-transcription.md) 和 [`transcription.md`](transcription.md)；关键词、Embedding、pgvector、Hybrid Search 与重建语义见 [`phase-6-search.md`](phase-6-search.md)。

> 生产就绪：Phase 1–8 完成只代表后端业务切片可运行，不代表产品已经可以直接上线。Auth、Web 真实数据、部署安全、真实云端与灾备的补全顺序及验收门槛见 [`production-readiness.md`](production-readiness.md)。

## 1. 后端定位

EchoNote 不是播客播放器，其后端不负责：

- 音频播放
- 播放进度同步
- 倍速和播放队列
- 播客客户端订阅体验
- 实时音频流媒体

EchoNote 后端主要负责：

```text
播客链接导入
↓
获取节目和音频信息
↓
保存用户随手记录
↓
云端转录与说话人处理
↓
Transcript 存储与阅读
↓
全文和语义搜索
↓
AI 总结与问答
↓
生成可分享到 Apple 备忘录的内容
```

整体采用：

```text
Go 模块化单体
+
API Server
+
Worker
+
PostgreSQL
+
对象存储
+
云端 ASR / LLM / Embedding
```

第一版不拆微服务。

---

# 2. 整体系统架构

```text
                       EchoNote PWA
                            │
                            │ HTTPS / SSE
                            ▼
                     EchoNote Go API
                            │
           ┌────────────────┼────────────────┐
           │                │                │
           ▼                ▼                ▼
        用户请求          数据查询          创建异步任务
      Notes/Episode      Search/AI              │
           │                │                   ▼
           └────────────────┼────────────── PostgreSQL
                            │                   │
                            │                   ▼
                            │             EchoNote Worker
                            │                   │
              ┌─────────────┼─────────────┬─────┴───────────┐
              ▼             ▼             ▼                 ▼
       Podcast Resolver  Object Store   Cloud ASR       LLM/Embedding
       Apple/RSS/小宇宙   OSS / S3       阿里云          Qwen等
              │             │             │                 │
              └─────────────┴─────────────┴─────────────────┘
                            │
                            ▼
                        PostgreSQL
              Transcript / Search / AI Results
```

后端编译成两个进程：

```text
echonote-api
echonote-worker
```

代码仍然在同一个 Go 项目中。

---

# 3. 核心功能模块

## 3.1 Auth：用户和会话

负责：

- 用户登录
- Session 管理
- 用户数据隔离
- 登录状态检查
- 退出登录

EchoNote 第一版以个人使用为主，建议采用：

```text
单用户优先
+
保留多用户数据结构
```

也就是第一版可以不开放注册，只通过命令行或初始化配置创建第一个用户，但所有核心表都保留：

```text
user_id
```

登录方式建议：

```text
邮箱或用户名
+
密码
+
HttpOnly Session Cookie
```

不要把长期 Token 放进浏览器 LocalStorage。

后续如果产品化，可以再增加：

- Apple 登录
- 邮箱验证码
- 多设备 Session 管理
- 订阅和额度

---

## 3.2 Imports：播客导入

负责接收：

```text
Apple Podcasts 链接
小宇宙链接
RSS Feed
直接 MP3 / M4A 链接
```

统一转换为 EchoNote 内部的 Episode。

### 导入流程

```text
用户提交链接
↓
创建 Import Job
↓
判断链接类型
↓
调用对应 Resolver
↓
获取 Podcast 和 Episode 元数据
↓
获取真实音频 URL
↓
查重
↓
创建或复用 Episode
↓
返回 Episode
```

### Resolver 接口

```go
type EpisodeResolver interface {
    CanResolve(rawURL string) bool

    Resolve(
        ctx context.Context,
        rawURL string,
    ) (*ResolvedEpisode, error)
}
```

实现：

```text
imports/resolvers/
├── apple.go
├── xiaoyuzhou.go
├── rss.go
└── direct_audio.go
```

统一结果：

```go
type ResolvedEpisode struct {
    SourceType      string
    ExternalID      string

    PodcastTitle    string
    PodcastAuthor   string
    PodcastCoverURL string

    EpisodeTitle    string
    Description     string
    PublishedAt     *time.Time
    DurationMS      int64

    CanonicalURL    string
    FeedURL         string
    AudioURL        string
}
```

### 去重策略

同一期节目可能从多个入口导入，因此按以下优先级去重：

```text
平台 Episode ID
↓
RSS GUID
↓
规范化 Audio URL
↓
标题 + 发布时间 + 时长
```

不能只用用户提交的 URL 去重。

---

## 3.3 Library：播客和单集资料库

负责：

- Podcast 列表
- Episode 列表
- 最近导入
- 转录状态展示
- 笔记数量统计
- 删除或归档节目
- 获取单集详情

核心关系：

```text
Podcast
└── Episode
    ├── Sources
    ├── Notes
    ├── Transcript
    ├── AI Artifacts
    └── Conversations
```

一个 Episode 可以同时存在：

```text
Apple Podcasts 来源
+
RSS 来源
+
官方网站来源
```

所以链接来源单独存储在：

```text
episode_sources
```

不要把所有来源字段直接塞进 `episodes`。

---

## 3.4 Notes：随手记录

这是 EchoNote 的核心功能之一。

用户可能正在 Apple Podcasts 或小宇宙里听节目，然后打开 EchoNote 记录一句话。

EchoNote 本身不知道用户播放到了哪里，所以第一版记录的是：

```text
用户创建笔记的时间
```

而不是：

```text
播客内部播放时间
```

### 创建笔记

```http
POST /api/v1/captures
```

请求：

```json
{
  "client_note_id": "38a3b021-4f61-4f88-a59d-4301fc45fa9b",
  "episode_id": "episode_123",
  "content": "这里关于 FDE 的定义和我以前理解的不一样",
  "created_at": "2026-08-20T19:32:00+09:00"
}
```

`client_note_id` 由 PWA 创建，用于离线同步和防止重复提交。

后端建立唯一约束：

```text
UNIQUE(user_id, client_note_id)
```

即使 PWA 因为网络问题重复提交，也只创建一条笔记。

### 未导入节目时记录

可以支持：

```json
{
  "client_note_id": "uuid",
  "episode_url": "https://podcasts.apple.com/...",
  "content": "这个观点后面需要重新梳理"
}
```

后端在同一个事务或业务流程中：

```text
创建 Pending Episode
+
保存 Note
+
创建 Import Job
```

这样用户不必等待节目识别完成才能记录。

---

## 3.5 Transcription：云端转录

负责：

- 音频下载
- 音频预处理
- 长音频切片
- 调用阿里云 ASR
- 轮询转录状态
- 保存原始结果
- 标准化 Transcript
- 跨 Chunk Speaker 对齐
- Transcript 去重合并
- 版本管理

转录档位：

```text
economy
→ Paraformer-v2

quality
→ Fun-ASR
```

具体实现采用前面确定的策略：

```text
较短音频
→ 单任务转录

长音频
→ 90 分钟 Core Window
→ 左右保留重叠区
→ 每段独立转录
→ 重叠区映射 Local Speaker
→ 生成 Global Speaker
→ Core Window 决定文本归属
```

这里必须保持两个概念：

```text
阿里云 speaker_id
=
Chunk Local Speaker ID
```

```text
EchoNote Speaker ID
=
整期节目范围的 Global Speaker ID
```

详细的切片和对齐算法作为：

```text
docs/architecture/transcription.md
```

单独维护，而整体后端只把它视为一个异步工作流。

---

## 3.6 Transcripts：转录文本管理

负责：

- 获取当前 Transcript
- Transcript 分页或分段加载
- Transcript 版本
- 说话人和时间戳
- 原始 ASR 结果关联
- 后续文本修正

一次重新转录不能覆盖旧结果。

结构：

```text
Episode
└── Transcript Version 1
└── Transcript Version 2
└── Transcript Version 3
```

其中只有一个：

```text
is_active = true
```

每个 Transcript Version 包含：

```text
Transcript Speakers
Transcript Segments
Provider
Model
Prompt/Hotword Config
Created At
```

每个 Segment：

```text
speaker
start_ms
end_ms
text
words
sequence
```

EchoNote 不负责播放，但仍然保存音频时间戳，因为它用于：

- 搜索结果定位
- AI 引用
- 对照原节目
- 说话人分析
- 未来跳转原播客

---

## 3.7 Speakers：说话人管理

负责：

- Global Speaker 创建
- Speaker 重命名
- Speaker 合并
- Speaker 角色
- 跨 Chunk 映射
- 未来声纹身份关联

第一版用户看到：

```text
Speaker A
Speaker B
Speaker C
```

用户可以修改为：

```text
主持人
朋新宇
嘉宾
```

### 修改名称

```http
PATCH /api/v1/transcripts/{transcript_id}/speakers/{speaker_id}
```

### 合并说话人

```http
POST /api/v1/transcripts/{transcript_id}/speakers/merge
```

例如：

```text
Speaker C
↓
合并到
Speaker A
```

合并后需要：

```text
更新 Transcript Segment
↓
更新搜索元数据
↓
使相关 AI Artifact 标记为可能过期
```

第一版不实现真正的跨节目声纹身份识别，但预留：

```text
speaker_profile_id
```

未来可以把不同节目里的 Speaker 关联到同一个人物。

---

## 3.8 Search：全文和语义搜索

搜索范围：

```text
用户笔记
+
Transcript
+
AI 整理结果
```

支持两个 Scope：

```text
单期节目
整个 Library
```

### 第一阶段：关键词搜索

个人数据量不大，直接使用 PostgreSQL。

中文搜索建议采用：

```text
精确子串匹配
+
pg_trgm 模糊匹配
```

不需要第一版就上 Elasticsearch。

### 第二阶段：语义搜索

加入：

```text
Embedding
+
pgvector
+
Hybrid Retrieval
```

统一搜索数据存储为：

```text
search_documents
```

文档类型：

```text
note
transcript
ai_artifact
```

Transcript 不按一句话生成一个向量，而是合并为语义块：

```text
约 300～600 个中文字符
+
少量重叠
```

每个 Search Chunk 保存：

```text
episode_id
document_type
speaker_id
start_ms
end_ms
text
embedding
```

Phase 6 实施澄清（2026-08-20）：语义块只合并同一 Speaker 的连续发言。若 Speaker 在达到 300 字前切换，保留短块，避免为了满足长度目标而丢失准确的 Speaker 和时间范围；长发言仍以约 600 字上限和最多约 80 字的完整 Segment 重叠切分。

### 混合排序

```text
关键词结果
+
向量结果
↓
RRF 或加权融合
↓
最终结果
```

搜索响应：

```json
{
  "type": "transcript",
  "episode_id": "episode_123",
  "episode_title": "E248｜一个“催发货”AI……",
  "speaker_name": "朋新宇",
  "start_ms": 1938000,
  "snippet": "真正复杂的企业智能体可能需要完成数百个步骤",
  "score": 0.91
}
```

---

## 3.9 AI：总结、梳理和问答

AI 模块分为两部分：

```text
AI Artifact
+
AI Conversation
```

### AI Artifact

用于生成固定结构的整理内容：

```text
一句话总结
核心观点
人物观点
值得回顾
结合我的笔记
```

不要只保存一大段 Markdown。

建议保存结构化 JSON：

```json
{
  "one_sentence_summary": "……",
  "key_points": [
    "……",
    "……"
  ],
  "speaker_views": [
    {
      "speaker_id": "speaker_a",
      "speaker_name": "朋新宇",
      "points": ["……"]
    }
  ],
  "note_connections": [
    "你的笔记中重点关注了……"
  ]
}
```

每份 AI Artifact 保存：

```text
artifact_type
model
prompt_version
transcript_version_id
notes_revision
input_hash
result_json
status
created_at
```

如果输入没有变化，重复请求直接返回缓存结果，避免重复产生费用。

### 生成策略

为控制成本，第一版建议：

```text
用户第一次打开 AI 页面
↓
按需生成
```

而不是每次转录完成后自动生成所有 AI 内容。

### 单期 AI 问答

输入范围：

```text
当前 Transcript
+
当前 Episode 的用户笔记
+
已有 AI Artifact
```

回答必须带引用：

```json
{
  "answer": "这期节目认为中国式 FDE 的差异主要体现在三个方面……",
  "citations": [
    {
      "transcript_segment_id": "segment_128",
      "speaker_name": "朋新宇",
      "start_ms": 1938000,
      "excerpt": "真正复杂的企业智能体……"
    }
  ]
}
```

后端只能允许模型引用本次检索返回的 Segment，不能让模型自行虚构 Segment ID。

AI 输出采用 SSE 流式返回，不需要 WebSocket。

### 跨 Library 问答

数据模型预留：

```text
conversation_scope = episode
conversation_scope = library
```

第一版只完成单期问答，之后再开放整个资料库问答。

### Phase 7 实施澄清

第一版将一句话总结、核心观点、人物观点、值得回顾和笔记关联合并为一个 `episode_summary` Artifact，以一次付费调用满足当前页面；需要独立刷新时再拆分类型。`notes_revision` 使用规范 Notes JSON 的 SHA-256 内容摘要，`input_hash` 覆盖 Transcript、Speaker、Segment 与 Notes 完整输入，避免多条写入路径维护计数器时漏增版本。

模型只返回已有来源 ID 和生成文本；Speaker 名称、原始 Quote、时间与 Note 内容由后端从本次输入补全。Artifact 增加派生 `search_text` 供 Phase 6 Search 重建，不作为业务真相。付费 Artifact Job 最多尝试一次，只有用户再次显式请求才会重试。

单期 Conversation 第一版只把 Transcript Segment 和 Note 作为可引用上下文，不把已有 Artifact 再喂给模型。Artifact 是派生内容且当前未逐项保存一级来源，作为上下文会放大旧错误；它仍进入普通 Search，待 Artifact 具备逐项 Citation 后再评估加入问答 Retrieval。

Conversation 的流式答案在末尾携带内部 Citation Envelope。后端只接受本次 Retrieval 白名单内的 Segment / Note ID，验证后才原子保存答案与 Citation。SSE 已发送的部分 Delta 无法撤回；验证失败时消息记为 `failed`、不发送 `done`，客户端必须使用新的 `client_message_id` 重试。

---

## 3.10 Exports：导出和分享到备忘录

EchoNote 后端不能直接写入 Apple 备忘录。

正确职责划分：

```text
后端
→ 生成适合分享的文本

PWA
→ 调用 iOS Share Sheet

用户
→ 选择 Apple 备忘录
```

接口：

```http
POST /api/v1/episodes/{episode_id}/exports
```

请求：

```json
{
  "mode": "organized_note",
  "include_user_notes": true,
  "include_summary": true,
  "include_key_points": true,
  "include_transcript": false
}
```

响应：

```json
{
  "title": "硅谷101｜E248 中国式 FDE",
  "text": "整理后的完整文本",
  "markdown": "# 硅谷101\n\n……",
  "suggested_filename": "硅谷101-E248.md"
}
```

第一版支持四种模式：

```text
仅我的笔记
AI 整理笔记
选中的 Transcript
完整 Transcript
```

导出本身可以是无状态操作，不需要一开始创建 `exports` 数据表。

### Phase 8 实施澄清

四种 API Mode 落地为 `notes_only`、`organized_note`、`selected_transcript` 与 `full_transcript`。一次响应同时返回 Apple Notes 友好的纯文本、Markdown 与安全建议文件名，PWA 继续负责 Clipboard / iOS Share Sheet；后端不创建 Apple Notes Provider。

Export 使用只读 `REPEATABLE READ` 快照，AI Section 只接受当前 ready Artifact，选中 Segment 必须全部属于 active Transcript。同步输出上限为 4 MiB，超过时明确返回 413 且不截断。第一版保持无状态，因此没有增加 Migration、Export 表、Job 或对象存储文件；需要异步大文件或历史下载链接时再扩展。

---

## 3.11 Settings：用户设置

建议保存：

```text
默认转录档位
默认说话人数提示
是否导入后自动转录
默认 AI 模型
默认导出格式
音频临时文件保留时长
界面语言
```

例如：

```json
{
  "default_transcription_profile": "economy",
  "auto_transcribe": false,
  "default_speaker_count_hint": null,
  "default_export_mode": "organized_note"
}
```

第一版建议：

```text
auto_transcribe = false
```

由用户确认后再产生云端转录费用。

---

# 4. 核心数据模型

```text
users
└── sessions

users
└── podcasts
    └── episodes
        ├── episode_sources
        ├── notes
        │
        ├── transcription_runs
        │   └── transcription_chunks
        │
        ├── transcript_versions
        │   ├── transcript_speakers
        │   └── transcript_segments
        │
        ├── search_documents
        │   └── search_chunks
        │
        ├── ai_artifacts
        │
        └── conversations
            └── messages
                └── message_citations

jobs
└── job_events
```

## 关键表

### `users`

```text
id
email
password_hash
status
created_at
updated_at
```

### `sessions`

```text
id
user_id
token_hash
expires_at
last_seen_at
created_at
```

### `podcasts`

```text
id
user_id
title
author
description
cover_url
feed_url
created_at
updated_at
```

### `episodes`

```text
id
user_id
podcast_id
title
description
published_at
duration_ms
cover_url

resolve_status
transcription_status
ai_status

created_at
updated_at
```

注意：

```text
resolve_status
transcription_status
ai_status
```

必须分开。

不能只设置一个笼统的：

```text
status
```

因为可能出现：

```text
节目解析成功
转录失败
AI 尚未生成
```

### `episode_sources`

```text
id
episode_id
source_type
external_id
source_url
canonical_url
audio_url
rss_guid
created_at
```

### `notes`

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

### `transcription_runs`

```text
id
episode_id
profile
provider
model
status
version
config JSONB
started_at
completed_at
error_code
error_message
created_at
```

### `transcript_versions`

```text
id
episode_id
transcription_run_id
version
is_active
status
created_at
```

### `transcript_speakers`

```text
id
transcript_version_id
stable_key
display_name
role
speaker_profile_id nullable
created_at
updated_at
```

### `transcript_segments`

```text
id
transcript_version_id
speaker_id
sequence
start_ms
end_ms
text
words JSONB
source_chunk_id
created_at
```

### `search_documents`

```text
id
user_id
episode_id
document_type
source_id
content
content_hash
metadata JSONB
created_at
updated_at
```

### `search_chunks`

```text
id
search_document_id
chunk_index
text
start_ms
end_ms
speaker_id
embedding
embedding_model
created_at
updated_at
```

`embedding_model` 是 Phase 6 为可重建索引增加的实现字段。模型发生变化时可只重建向量，不需要改变业务源数据。

### `ai_artifacts`

```text
id
user_id
episode_id
transcript_version_id
artifact_type
model
prompt_version
input_hash
status
result JSONB
created_at
updated_at
```

### `conversations`

```text
id
user_id
episode_id nullable
scope
title
created_at
updated_at
```

### `messages`

```text
id
conversation_id
role
content
model
created_at
```

### `message_citations`

```text
id
message_id
transcript_segment_id nullable
note_id nullable
excerpt
created_at
```

---

# 5. 核心 API

统一前缀：

```text
/api/v1
```

## Auth

```http
POST /auth/login
POST /auth/logout
GET  /me
```

## Imports

```http
POST /imports
GET  /imports/{import_id}
```

## Library

```http
GET    /episodes
GET    /episodes/{episode_id}
DELETE /episodes/{episode_id}
```

## Notes

```http
POST   /captures
GET    /episodes/{episode_id}/notes
POST   /episodes/{episode_id}/notes
PATCH  /notes/{note_id}
DELETE /notes/{note_id}
```

## Transcription

```http
POST /episodes/{episode_id}/transcriptions
GET  /transcriptions/{run_id}
GET  /transcriptions/{run_id}/events
POST /transcriptions/{run_id}/retry
POST /transcriptions/{run_id}/cancel
```

## Transcript

```http
GET /episodes/{episode_id}/transcript
GET /transcripts/{transcript_id}/segments
```

## Speakers

```http
PATCH /transcripts/{transcript_id}/speakers/{speaker_id}
POST  /transcripts/{transcript_id}/speakers/merge
```

## Search

```http
GET /search?q=FDE&scope=library
GET /search?q=融资&scope=episode&episode_id=episode_123
POST /search/reindex
```

`POST /search/reindex` 是 Phase 6 增加的显式运维入口，用于全 Library 或单 Episode 重建，也用于在一次不自动重试的付费 Embedding Job 失败后由用户明确重试。

## AI

```http
POST /episodes/{episode_id}/ai/artifacts
GET  /episodes/{episode_id}/ai/artifacts

POST /conversations
GET  /conversations/{conversation_id}
POST /conversations/{conversation_id}/messages
```

最后一个接口使用 SSE 流式返回。

## Export

```http
POST /episodes/{episode_id}/exports
```

## Settings

```http
GET   /settings
PATCH /settings
```

---

# 6. 异步任务系统

以下任务不能在 HTTP 请求中同步执行：

```text
播客解析
音频下载
音频转码
长音频切片
ASR 转录
Speaker 对齐
Transcript 合并
搜索索引
Embedding
AI 总结
清理临时音频
```

第一版使用 PostgreSQL Job Queue，不需要 Redis。

## `jobs`

```text
id
user_id
type
entity_type
entity_id
payload JSONB

status
stage
priority

attempt
max_attempts
run_after

locked_by
locked_at

error_code
error_message

created_at
updated_at
completed_at
```

领取任务：

```sql
SELECT *
FROM jobs
WHERE status = 'queued'
  AND run_after <= NOW()
ORDER BY priority DESC, created_at
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

## 任务类型

```text
resolve_episode
download_audio
prepare_audio
plan_transcription
render_audio_chunk
submit_asr
poll_asr
ingest_asr_result
align_speakers
merge_transcript
build_keyword_index
generate_embeddings
generate_ai_artifact
cleanup_audio
```

## 任务原则

每个 Job 必须：

- 幂等
- 可重试
- 可以单独失败
- 记录明确错误代码
- Worker 重启后可以恢复
- 不重复产生云端费用

### 不要在 Worker 中持续等待

例如 ASR 任务：

```text
submit_asr
↓
保存 external_task_id
↓
当前 Job 完成
↓
创建 5 秒后的 poll_asr Job
```

`poll_asr` 如果仍在运行：

```text
更新 run_after
↓
重新入队
```

不要用一个 Goroutine `sleep` 几分钟。

---

# 7. 任务进度

前端通过：

```http
GET /api/v1/transcriptions/{run_id}/events
```

建立 SSE。

事件示例：

```text
episode_resolved
audio_download_started
audio_prepared
chunks_planned
chunk_transcription_started
chunk_transcription_completed
speaker_alignment_started
transcript_merged
search_index_built
completed
```

如果云服务不提供真实百分比，不要伪造：

```text
63%
```

可以显示：

```text
正在转录第 2/3 段
```

或者：

```text
正在整理转录结果
```

---

# 8. Provider 设计

外部服务都要抽象，但只在真正存在替换需求的位置抽象。

## Podcast Resolver

```go
type EpisodeResolver interface {
    CanResolve(url string) bool
    Resolve(ctx context.Context, url string) (*ResolvedEpisode, error)
}
```

## ASR Provider

```go
type ASRProvider interface {
    Submit(
        ctx context.Context,
        req TranscriptionRequest,
    ) (*ExternalTask, error)

    Poll(
        ctx context.Context,
        taskID string,
    ) (*ExternalTaskStatus, error)

    FetchResult(
        ctx context.Context,
        resultURL string,
    ) (*RawASRResult, error)
}
```

## LLM Provider

```go
type LLMProvider interface {
    GenerateStructured(
        ctx context.Context,
        req StructuredGenerationRequest,
    ) (*StructuredGenerationResult, error)

    StreamChat(
        ctx context.Context,
        req ChatRequest,
    ) (<-chan ChatEvent, error)
}
```

## Embedding Provider

```go
type EmbeddingProvider interface {
    Embed(
        ctx context.Context,
        texts []string,
        inputType EmbeddingInputType,
    ) ([][]float32, error)

    Model() string
    Dimensions() int
}
```

Phase 6 将原草案接口补充为区分 `query` 与 `document`，并暴露模型和维度。原因是当前阿里云 Embedding API 明确建议非对称检索区分两种输入；模型/维度则用于验证 Provider 响应并选择兼容的已存向量。业务层仍只依赖 Provider 接口。

## Object Store

```go
type ObjectStore interface {
    Put(ctx context.Context, key string, reader io.Reader) error
    Delete(ctx context.Context, key string) error
    SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
```

第一版只实现当前需要的 Provider，不要为了抽象一次性写五家适配器。

---

# 9. Go 技术栈

建议：

```text
HTTP Router
go-chi/chi

Database Driver
pgx

SQL Code Generation
sqlc

Migrations
golang-migrate

Logging
标准库 log/slog

Validation
go-playground/validator

API Contract
OpenAPI + oapi-codegen

Testing
testing + httptest + testcontainers-go
```

数据库：

```text
PostgreSQL
+
pgvector
+
pg_trgm
```

第一版不使用：

- Redis
- Kafka
- Elasticsearch
- Temporal
- Kubernetes
- 微服务

---

# 10. 项目文件结构

```text
echonote/
├── apps/
│   ├── web/
│   │   └── EchoNote PWA
│   │
│   └── server/
│       ├── cmd/
│       │   ├── api/
│       │   │   └── main.go
│       │   ├── worker/
│       │   │   └── main.go
│       │   └── migrate/
│       │       └── main.go
│       │
│       ├── internal/
│       │   ├── app/
│       │   │   ├── api.go
│       │   │   ├── worker.go
│       │   │   └── dependencies.go
│       │   │
│       │   ├── modules/
│       │   │   ├── auth/
│       │   │   ├── imports/
│       │   │   ├── library/
│       │   │   ├── notes/
│       │   │   ├── transcription/
│       │   │   ├── transcripts/
│       │   │   ├── speakers/
│       │   │   ├── search/
│       │   │   ├── ai/
│       │   │   ├── exports/
│       │   │   ├── settings/
│       │   │   └── jobs/
│       │   │
│       │   ├── platform/
│       │   │   ├── postgres/
│       │   │   ├── objectstore/
│       │   │   ├── ffmpeg/
│       │   │   ├── httpclient/
│       │   │   ├── security/
│       │   │   └── logging/
│       │   │
│       │   └── shared/
│       │       ├── errors/
│       │       ├── ids/
│       │       ├── pagination/
│       │       └── clock/
│       │
│       ├── migrations/
│       ├── openapi/
│       │   └── openapi.yaml
│       ├── go.mod
│       └── go.sum
│
├── docs/
│   ├── product/
│   │   ├── vision.md
│   │   └── requirements.md
│   ├── design/
│   └── architecture/
│       ├── overview.md
│       ├── database.md
│       ├── imports.md
│       ├── transcription.md
│       ├── search.md
│       └── ai.md
│
├── deployments/
│   ├── docker/
│   └── caddy/
│
├── docker-compose.yml
├── Makefile
├── .env.example
└── README.md
```

每个功能模块内部采用相同结构：

```text
modules/notes/
├── handler.go
├── service.go
├── repository.go
├── models.go
├── errors.go
└── queries.sql
```

有异步任务的模块再增加：

```text
jobs.go
worker.go
```

有外部 Provider 的模块再增加：

```text
providers/
```

例如：

```text
modules/transcription/
├── handler.go
├── service.go
├── workflow.go
├── repository.go
├── models.go
├── audio/
├── alignment/
├── merge/
├── providers/
│   └── aliyun/
└── jobs/
```

---

# 11. 数据一致性和版本策略

## Transcript 不可直接覆盖

每次重新转录：

```text
创建新的 Transcript Version
↓
处理完成
↓
切换为 Active
↓
旧版本保留
```

## AI 结果和输入绑定

AI Artifact 保存：

```text
transcript_version_id
notes_revision
prompt_version
model
input_hash
```

Transcript 或笔记发生变化后，旧 AI 结果标记：

```text
stale
```

而不是直接删除。

## 搜索索引可重建

Search Document 保存：

```text
content_hash
```

只有内容变化时重新生成 Embedding。

## 笔记同步幂等

依靠：

```text
client_note_id
```

防止弱网重复创建。

## 云端任务幂等

转录 Chunk 使用：

```text
audio_hash
+
时间区间
+
模型
+
配置
```

生成任务指纹，避免重复计费。

---

# 12. 安全要求

## 外部链接安全

导入和下载音频必须防止 SSRF：

- 仅允许 HTTP/HTTPS
- 禁止内网 IP
- 禁止回环地址
- DNS 解析后重新校验
- 限制重定向次数
- 限制下载体积
- 设置连接和读取超时
- 校验音频 Content-Type
- 禁止访问本地文件协议

## 身份与权限

每个查询都必须包含：

```text
user_id
```

不能只凭：

```text
episode_id
```

读取数据。

## Session

使用：

```text
Secure
HttpOnly
SameSite
```

Cookie。

## 密钥

云服务密钥：

- 只保存在服务器环境变量或密钥管理服务
- 不返回给 PWA
- 不写日志
- 不入 Git
- 不明文保存到普通业务表

## AI 安全

Transcript 和 Notes 都属于用户数据，不应被当作系统指令。

AI Prompt 中应明确区分：

```text
系统规则
用户问题
检索资料
```

避免播客文本中的内容影响系统行为。

---

# 13. 文件存储和清理

建议生命周期：

```text
原始下载音频
任务完成后删除

切片音频
保留 72 小时

云 ASR 原始 JSON
长期保存

标准化 Transcript
长期保存

Embedding
长期保存，可重建

导出文件
按需生成，不长期保存
```

对象存储路径：

```text
users/{user_id}/
└── episodes/{episode_id}/
    ├── source/
    ├── transcription-runs/{run_id}/
    │   ├── chunks/
    │   └── raw-results/
    └── exports/
```

---

# 14. 日志和监控

所有日志必须带：

```text
request_id
user_id
episode_id
job_id
transcription_run_id
provider
```

关键指标：

```text
播客解析成功率
音频下载失败率
转录成功率
平均转录时长
每小时音频转录成本
Speaker 对齐低置信度比例
搜索耗时
Embedding 调用量
LLM Token 成本
AI 问答耗时
Job 重试次数
```

错误码要稳定，例如：

```text
IMPORT_UNSUPPORTED_URL
IMPORT_EPISODE_NOT_FOUND
AUDIO_DOWNLOAD_FAILED
AUDIO_TOO_LARGE
ASR_SUBMIT_FAILED
ASR_RESULT_EXPIRED
SPEAKER_ALIGNMENT_UNCERTAIN
AI_PROVIDER_FAILED
SEARCH_INDEX_NOT_READY
```

前端根据错误码决定是否展示重试入口。

---

# 15. 部署方案

Docker Compose：

```text
echonote-web
echonote-api
echonote-worker
postgres
```

本地开发可以增加：

```text
minio
```

生产环境使用：

```text
阿里云 OSS
或
兼容 S3 的对象存储
```

推荐同域部署：

```text
https://echonote.example.com/
→ PWA

https://echonote.example.com/api/
→ Go API
```

这样可以减少：

- CORS
- Cookie 跨域
- Safari PWA 登录问题

反向代理可以使用 Caddy。

Worker 可以水平扩容：

```text
worker-1
worker-2
worker-3
```

由于 Job Queue 使用 PostgreSQL 锁，不会重复领取同一个任务。

---

# 16. 推荐开发顺序

## 第一阶段：项目基础

实现：

```text
Go 项目骨架
PostgreSQL
Migrations
OpenAPI
Auth
用户 Session
Job Queue
对象存储接口
```

完成标准：

- API 和 Worker 可以独立启动
- Worker 重启后任务不会丢失
- 用户只能访问自己的数据

---

## 第二阶段：导入和记录

实现：

```text
Apple Podcasts Resolver
RSS Resolver
直接音频 Resolver
Podcast / Episode
快速记录 Notes
离线幂等同步
Library API
```

完成标准：

- 用户粘贴 Apple Podcasts 链接后能创建 Episode
- 解析尚未完成时也可以记录笔记
- 重复导入同一期不会产生重复 Episode

---

## 第三阶段：完整转录

实现：

```text
阿里云 ASR Provider
音频预处理
单文件转录
长音频切片
重叠 Speaker 对齐
Transcript Version
Speaker 重命名和合并
```

完成标准：

- 3 小时播客可以完整转录
- Chunk 之间 `speaker_id` 交换后仍能映射
- 单个 Chunk 失败可以单独重试
- 最终 Transcript 不重不漏

---

## 第四阶段：搜索

实现：

```text
Notes 关键词搜索
Transcript 关键词搜索
Search Document
Embedding Provider
pgvector
Hybrid Search
```

完成标准：

- 可以同时搜索 Transcript 和自己的笔记
- 搜索结果返回节目、Speaker、时间和摘要
- 索引可以重建

---

## 第五阶段：AI

实现：

```text
AI Artifact
一句话总结
核心观点
人物观点
单期 AI 问答
引用验证
SSE 流式输出
```

完成标准：

- AI 回答必须带可核对引用
- Transcript 不变时不会重复生成摘要
- 笔记修改后 AI 结果会被标记为过期

---

## 第六阶段：导出和完善

实现：

```text
导出模板
分享到 Apple Notes 的文本生成
任务事件
错误恢复
成本统计
数据备份
音频生命周期清理
```

---

# 17. MVP 范围

第一版必须完成：

1. 用户登录。
2. Apple Podcasts、RSS 和直接音频导入。
3. Episode 资料库。
4. 快速记录笔记。
5. Paraformer-v2 和 Fun-ASR 两档转录。
6. 长音频切片及跨 Chunk Speaker 对齐。
7. Speaker 重命名和合并。
8. Transcript 阅读接口。
9. Transcript 和 Notes 搜索。
10. 单期 AI 总结。
11. 单期 AI 问答和引用。
12. 生成分享到 Apple 备忘录的文本。
13. 异步任务重试和恢复。
14. 用户数据隔离。

第一版不做：

- 播放器
- 播放记录
- 播客订阅自动更新
- 实时语音转录
- 真正跨节目的声纹身份识别
- 整个知识库 AI 问答
- 团队协作
- 计费系统
- App Store 原生应用
- 微服务
- Elasticsearch
- Kafka
- Kubernetes

---

# 18. 最终技术方案

```text
前端
React + TypeScript + Vite PWA

后端
Go + Chi

数据库
PostgreSQL

数据库访问
pgx + sqlc

数据库扩展
pgvector + pg_trgm

异步任务
PostgreSQL Job Queue

对象存储
阿里云 OSS / S3 Compatible

音频处理
FFmpeg

转录
Paraformer-v2 / Fun-ASR

AI
统一 LLM Provider

Embedding
统一 Embedding Provider

实时进度
SSE

API 契约
OpenAPI

部署
Docker Compose + Caddy
```

EchoNote 后端最核心的划分应该是：

```text
imports
解决“把节目放进来”

notes
解决“把想法记下来”

transcription
解决“把声音转出来”

speakers
解决“区分是谁说的”

search
解决“把内容找出来”

ai
解决“把内容理解清楚”

exports
解决“把结果沉淀出去”
```

这才是完整的 EchoNote 后端，而不是围绕转录模块搭建的后端。
