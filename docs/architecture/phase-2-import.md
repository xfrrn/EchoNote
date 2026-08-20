# Phase 2：播客导入实施记录

更新时间：2026-08-20

## 范围

本阶段按当前任务定义，只完成 Import 垂直切片：

- Apple Podcasts Resolver
- RSS Resolver
- Direct Audio Resolver
- Podcast、Episode、Episode Source 创建
- Import API 与 `resolve_episode` Worker Job
- 跨来源 Episode 去重

没有提前实现 Phase 3 Library、Phase 4 Notes、转录、Search、AI、Export，也没有修改仍使用 Mock Data 的 Web Demo。

## 调用链

```text
POST /api/v1/imports
→ 同一 PostgreSQL 事务创建 Import、Job、queued Event
→ Worker 领取 resolve_episode
→ Apple / Direct Audio / RSS Resolver
→ 事务锁定身份键并查重
→ 创建或补全 Podcast / Episode
→ 保存 Episode Source 与身份键
→ Import 关联 Episode
→ Job succeeded
→ GET /api/v1/imports/{id} 返回 Episode ID
```

API 契约以 `apps/server/openapi/openapi.yaml` 为准：

```http
POST /api/v1/imports
Content-Type: application/json

{"url":"https://podcasts.apple.com/.../id123?i=456"}
```

创建请求返回 `202 queued`。客户端通过 `GET /api/v1/imports/{import_id}` 查询 `queued`、`running`、`succeeded`、`failed` 或 `canceled`；成功时返回 `episode_id`，失败时返回稳定的错误码与安全错误信息。

## Provider 行为

### Apple Podcasts

- 只接受 `podcasts.apple.com` 与旧版 `itunes.apple.com` 链接。
- 链接必须同时包含节目 `id` 和单集查询参数 `i`；只有节目 ID 无法确定要导入哪一期，因此异步失败为 `IMPORT_INVALID_APPLE_URL`。
- 使用 Apple iTunes Lookup API 获取节目、单集、RSS GUID 和真实 enclosure URL，不解析 Apple 页面 HTML。
- Lookup 每次最多读取 200 个单集；找不到目标时返回 `IMPORT_EPISODE_NOT_FOUND`，不猜测或误导入最新一期。

实现依据：[Apple Podcasts RSS 要求](https://podcasters.apple.com/support/823-podcast-requirements) 与 [iTunes Search API Lookup 文档](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/LookupExamples.html)。

### RSS

- 解析 RSS 2.0，不额外引入 RSS 库。
- Feed URL 本身不包含单集标识，因此选择带有效音频 enclosure、`pubDate` 最新的一期；日期无法解析时保持 Feed 顺序。
- 支持常见 RFC 822/1123/RFC 3339 日期和 `SS`、`MM:SS`、`HH:MM:SS` 时长。
- RSS GUID、enclosure、节目/单集标题、描述、作者、封面统一映射为 `ResolvedEpisode`。
- 当前不支持 Atom；Apple 已不接受新的 Atom Podcast Feed，本阶段范围也是 RSS Resolver。

### Direct Audio

- 支持 MP3、M4A、AAC、WAV、FLAC、OGG、Opus 等直接 HTTP(S) 音频链接。
- 优先发送 `HEAD`；服务不支持时才发送 `Range: bytes=0-0`，不会在导入阶段下载完整音频。
- 必须是 `audio/*`，或带已知音频扩展名的 `application/octet-stream`；标题取 `Content-Disposition` 文件名或 URL 文件名。
- 直接音频没有可靠 Podcast 元数据，因此创建的 `episodes.podcast_id` 允许为空，不创建虚构 Podcast。

## 数据库 Migration v2

新增：

- `podcasts`
- `episodes`
- `episode_sources`
- `episode_identity_keys`
- `imports`

`episodes` 分别保存 `resolve_status`、`transcription_status` 与 `ai_status`，没有合并成笼统状态。`episode_sources` 独立保存来源，同一个 Episode 可以同时保留 Apple、RSS 和直接音频来源。

`imports.job_id` 在表结构上允许短暂为空，因为 Import 必须先取得数据库生成的 ID，Job 才能以它作为 `entity_id`；Repository 在一个事务内创建 Import、Job、Event 并回填 `job_id`，未回填时不会提交。

## 去重与并发

严格按设计优先级生成固定长度 SHA-256 身份键：

```text
Apple Episode ID
→ RSS GUID
→ 规范化 Audio URL
→ 规范化标题 + 发布时间 + 时长
```

身份键存储在 `episode_identity_keys`，主键为 `(user_id, identity_key)`。落库前按排序后的身份键获取 PostgreSQL transaction advisory lock，再按原优先级查找已有 Episode，避免两个 Worker 从不同入口并发导入时重复创建。URL 规范化只处理 scheme/host 大小写、默认端口、fragment 与 query 顺序，不删除可能影响签名的 query 参数。

## 外链安全

生产 Resolver 共用受限 HTTP Client：

- 仅允许 HTTP/HTTPS，禁止 URL credentials、本地文件协议、localhost 和 `.local`。
- IP literal 与 DNS 解析结果都拒绝私网、回环、链路本地、非全局单播和 CGNAT 地址。
- 校验每次重定向，最多 5 次；禁用环境代理，连接到已经校验的解析 IP，防止 DNS rebinding。
- 总请求、连接、TLS 和响应头均有超时。
- Metadata/RSS 响应最多读取 5 MiB。
- Direct Audio 只探测 Content-Type，不下载完整文件。

确定性输入错误第一次即失败；网络、429 和 5xx 错误仍由 PostgreSQL Job Queue 有限重试。Worker 只把稳定错误码和安全消息暴露给 Import API。

## 设计调整记录

### 1. 当前 Phase 顺序优先

整体方案原“第二阶段”同时列出 Import、Library 和 Notes；当前任务明确把它们拆成 Phase 2、3、4。本阶段只实现 Import，避免留下跨阶段半成品。

### 2. Auth 前的用户边界

Auth 不在当前阶段，但设计要求所有业务查询带 `user_id`。新增业务表的 `user_id` 均为非空，API 暂时使用 `ECHONOTE_USER_ID` 指定的单用户 UUID；开发默认值为 `00000000-0000-4000-8000-000000000001`。Worker 从 Job 读取用户 ID，不使用全局无条件查询。

影响：这是单用户运行模式，不是登录机制。进入 Auth 阶段后由 Session 提供用户 ID，并通过后续 migration 添加 `users` 外键；Import Repository 与 Worker 的方法签名无需改变。

### 3. `ResolvedEpisode` 补充字段

整体方案示例没有列出 `RSSGUID` 与 `PodcastDescription`，但去重策略和 `podcasts.description` 明确需要它们，因此统一 Provider 结果增加这两个字段。

### 4. Apple 与 RSS 的歧义不猜测

Apple 只有节目链接时拒绝导入；RSS Feed 没有标准单集 ID 时明确选择最新一期。两种行为都记录在本文件和 API 使用说明中，避免隐含、不稳定的选择。

## 验证

```bash
go generate ./...
go test ./...
go vet ./...
```

设置隔离数据库后，集成测试应用全部 Migration，并发执行两个来源的落库，并验证：

```text
Apple source + RSS source
→ 相同 RSS GUID / Audio URL
→ 1 个 Episode + 2 条 Episode Source
```

2026-08-20 已在独立的本地 `echonote` 数据库（PostgreSQL 18，无 Docker）完成 Migration v2 与集成测试。另以真实 Apple 单集链接执行两次完整 API → Worker → Apple Lookup → PostgreSQL 流程；两次均为 `succeeded` 且返回同一个 Episode ID。端到端临时数据随后按精确 ID 删除。

## 下一阶段入口

Phase 3 在现有 `podcasts`、`episodes`、`episode_sources` 上实现 Library 查询、详情、删除与三个独立状态的展示，不需要重做 Import。
