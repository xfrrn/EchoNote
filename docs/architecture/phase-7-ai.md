# Phase 7：AI 实施记录

更新时间：2026-08-21

## 范围

本阶段按垂直切片完成：

- 按需生成 Episode AI Artifact
- 一句话总结、核心观点、人物观点、值得回顾、笔记关联
- Artifact 缓存、输入绑定、过期标记与 Search 入索引
- 单期 Episode Conversation
- Notes + active Transcript 的 Hybrid Retrieval
- 可核对 Citation、消息幂等与 SSE 流式响应
- 阿里云百炼 Qwen LLM Provider

没有实现跨 Library 问答、自动后台总结、前端 Mock Data 切换或 Export。`library` Conversation Scope 仅在数据库和 OpenAPI 枚举中预留，创建接口本阶段明确拒绝该 Scope。

## 实现前分析

Phase 6 已提供 Notes、active Transcript、Search Document、Hybrid Search 与 PostgreSQL Job Queue。因此最小可靠路径是复用已有真相表和检索链路：

```text
用户显式请求 Artifact
→ 锁定 Episode 并计算输入摘要
→ 命中缓存则直接返回
→ 否则 enqueue generate_ai_artifact
→ Worker 调用 LLM Provider
→ 后端校验全部引用 ID
→ 持久化结构化结果并重建 Search
```

```text
用户发送 Conversation Message
→ client_message_id 幂等占位
→ Phase 6 Hybrid Search
→ Search Chunk 映射回精确 Segment / Note
→ LLM 原生 SSE
→ 后端隐藏并解析 Citation Envelope
→ 白名单校验成功后原子保存 Answer + Citations
```

没有增加 Redis、Kafka、向量服务、WebSocket 或新的基础设施。

## Migration v6

`000006_ai` 增加：

```text
ai_artifacts
conversations
messages
message_citations
```

`ai_artifacts` 保存模型、Prompt 版本、active Transcript Version、Notes 摘要、完整输入摘要、结构化结果、派生搜索文本、Token 用量和状态。部分唯一索引保证同一 Episode / Artifact Type 最多只有一份 `queued | generating | ready` 结果，历史 `stale | failed` 结果保留用于审计和缓存复用。

`messages` 通过 `(conversation_id, client_message_id)` 保证用户消息幂等，并用 `reply_to_message_id` 把每条 Assistant Message 绑定到对应 User Message。查询按轮次和角色稳定排序，不依赖同一事务内相同的时间戳或随机 UUID。

`message_citations` 必须且只能关联一个 Transcript Segment 或 Note。Episode 删除通过外键级联清理 Artifact、Conversation、Message 与 Citation；删除前会取消相关 AI Job。

## Artifact 与设计调整

本阶段只实现一种 `episode_summary` Artifact，一次 Provider 调用返回前端当前需要的五类结构：

```text
one_sentence_summary
key_points
speaker_views
worth_reviewing
note_connections
```

这样避免同一输入为了五张卡片产生五次费用。若未来不同卡片需要独立刷新或模型，才拆分新的 `artifact_type`。

设计中的 `notes_revision` 落地为排序后 Notes JSON 的 SHA-256 摘要，而不是额外维护可变计数器。原因是 Notes 已有离线幂等、软删除和 Capture 合并路径，内容摘要能直接代表真实输入，减少跨路径漏增版本的风险。`input_hash` 则覆盖标题、Transcript Version、Speaker、Segment 与 Notes 的完整规范 JSON。

模型只返回已有 `speaker_id`、`transcript_segment_id`、`note_id` 及生成文本。Speaker Name、时间、原始 Quote 和 Note 内容由后端从本次输入补全；未知 ID、重复 ID、未知字段、超限内容或尾随 JSON 都会使 Artifact 失败，不能写成可信结果。

为了接入 Phase 6 Search，Artifact 额外保存可重建的 `search_text` 派生字段。只有最新 `ready` Artifact 会进入 `ai_artifact` Search Document；标记 `stale` 后下次 Search 重建会删除旧索引。

以下业务变化会在原事务中锁定 Episode、标记当前 Artifact 为 `stale`、取消未完成 AI Job，并把 Episode `ai_status` 改回 `waiting`：

- Note 创建、编辑、软删除
- Capture 解析并合并到目标 Episode
- 新 Transcript Version 激活
- Speaker 重命名或合并

旧结果不删除。若输入后来恢复为完全相同的摘要、模型与 Prompt，显式请求会重新激活缓存，不再次调用 LLM。

## LLM Provider 与费用边界

业务层只依赖：

```go
GenerateStructured(ctx, request)
StreamChat(ctx, request)
Model()
```

当前 Provider 使用阿里云百炼 OpenAI 兼容 Chat Completions：

```text
endpoint = https://dashscope.aliyuncs.com/compatible-mode/v1
model    = qwen-plus（可配置）
Artifact = response_format: json_object
Chat     = stream: true + include_usage
```

Endpoint 必须是无 URL 凭据的 HTTPS 地址；API Key 只从环境变量读取，不写入数据库、响应或日志。HTTP Client 复用项目已有公网目标校验，并限制普通响应、单个 SSE Event 与完整 Stream 大小。

`generate_ai_artifact` 的 `max_attempts=1`。LLM 请求可能已被 Provider 计费但响应丢失，因此 Queue 不自动重放；失败后只有新的显式 POST 才会重试。相同输入的 ready Artifact 直接返回 200，queued / generating 返回 202。

实现依据：

- [阿里云百炼 Chat Completions](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-chat-completions)
- [阿里云百炼 JSON 结构化输出](https://www.alibabacloud.com/help/en/model-studio/qwen-structured-output)
- [阿里云百炼流式输出](https://www.alibabacloud.com/help/en/model-studio/stream)
- [阿里云百炼模型列表](https://www.alibabacloud.com/help/en/model-studio/models)

## Conversation、Citation 与 SSE

本阶段 Conversation 必须绑定用户拥有且已有 active Transcript 的 Episode。Retrieval 复用 Phase 6 Search，最多取 12 个、总计约 20,000 字符的可信来源。Transcript Search Chunk 会映射回组成它的原始 Segment，因此 Citation 永远落在真实 Segment 边界；结果不足时补充当前 Episode 的少量 Segment / Note。

整体方案曾建议把已有 AI Artifact 也作为问答输入。本阶段刻意只把 Transcript Segment 与 Note 放入可引用上下文：Artifact 是派生结果，不是一级证据，继续喂给模型会放大旧总结错误，且无法为每个句子恢复可靠来源。Artifact 仍参与普通 Search；等 Artifact 自身保存逐项来源后，再考虑把它加入 Conversation Retrieval。该调整不减少单期问答能力，并强化了 Citation 的可核对性。

Provider 必须在答案末尾输出内部协议：

```text
<ECHONOTE_CITATIONS>{"ids":["segment:...","note:..."]}</ECHONOTE_CITATIONS>
```

该 Envelope 不转发给客户端。后端允许引用的 ID 仅来自本次 Retrieval 白名单，并拒绝虚构 ID、尾随数据、不完整 Envelope、空引用或超限输出。验证成功后，Answer 与 Citation 在一个数据库事务中保存；响应依次发送 `delta`、`citation`、`done` 事件。

SSE 在最终 Citation 校验前已经发送的 Answer Delta 无法撤回。若 Provider 中断、客户端断开或引用无效，Assistant Message 记为 `failed`，发送 `error`，不发送 `done`，也不保存不可信 Answer。客户端重试必须使用新的 `client_message_id`；对已完成消息重发相同 ID 和内容会回放持久化结果，不调用 Provider。相同 ID 配不同内容返回 409。

失败轮次的 User Message 不进入后续模型历史，避免只有问题、没有可信回答的半轮污染上下文。

## API

OpenAPI 0.7.0 新增：

```text
GET  /api/v1/episodes/{episode_id}/ai/artifacts
POST /api/v1/episodes/{episode_id}/ai/artifacts
POST /api/v1/conversations
GET  /api/v1/conversations/{conversation_id}
POST /api/v1/conversations/{conversation_id}/messages
```

最后一个接口返回 `text/event-stream`。事件格式：

```text
delta     {"text":"...","replayed":false}
citation  {"source_type":"transcript|note", ...}
done      {"message_id":"...","replayed":false}
error     {"code":"AI_STREAM_FAILED","message":"..."}
```

未配置 LLM 时，Artifact 请求与发送消息返回 503；Artifact 读取和已保存 Conversation 读取仍可用。

## 安全

- 每个 Artifact、Conversation、检索查询和持久化更新都包含 `user_id`。
- Transcript / Notes 在 Prompt 中标记为不可信数据，与系统规则、用户问题明确分区。
- Provider 输出不是业务真相；结构、长度、来源 ID 和 Token 数均由后端校验。
- Citation 保存原文快照，同时保留 Segment / Note 外键。
- 不记录 Prompt、Episode 内容、密钥或完整 Provider 响应。

## 验收

自动验收覆盖：

- Migration v6 真实创建与回滚。
- Qwen JSON 与 SSE 请求格式、鉴权、Token 用量和完成标记。
- Artifact 结构校验、未知 ID 拒绝、输入摘要与 Search Text。
- Artifact 生成、缓存命中、Note 变化后 stale、Search 加入和移除。
- Episode Conversation 创建、真实 Segment Citation、SSE、持久化与稳定消息顺序。
- `client_message_id` 成功回放、内容冲突与失败后新 ID 语义。
- 虚构 Citation 拒绝，失败轮次不进入历史。
- 跨用户 Conversation 返回 404。
- Phase 1–6 全量 PostgreSQL 回归。

本地测试只使用独立 `echonote` 数据库和 Fake LLM Provider，没有访问 `autoup`、没有启动 Docker、没有调用真实阿里云接口，因此未产生云端费用。部署前仍需用目标百炼账号做一次小额度受控 Smoke Test。
