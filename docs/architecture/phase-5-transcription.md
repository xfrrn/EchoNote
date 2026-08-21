# Phase 5：Transcription 实施记录

更新时间：2026-08-20

## 范围

本阶段按垂直切片完成：

- 安全音频下载、FFmpeg 预处理与 Chunk 渲染
- 阿里云 Paraformer-v2 / Fun-ASR 异步 Provider
- 阿里云 OSS Object Storage Provider
- PostgreSQL Job Queue 的无损延迟轮询
- 长音频切片、Speaker Alignment 与 Transcript 合并
- Transcript Version、Segment、Speaker 重命名和合并
- 转录状态、失败重试、取消、SSE 事件与对象生命周期

没有实现播放器、音频流、播放历史，也没有提前实现 Search、Embedding、AI 或 Export。Web Demo 本阶段仍使用 Mock Data；后端契约以 OpenAPI 0.5.0 为准。

## 实现前结论与文档补全

整体方案明确引用 `docs/architecture/transcription.md`，但该文件在当前仓库、Git 历史和远端都不存在。为了不自行改变算法，本阶段只把整体方案已经固定的内容补成可运行参数：

```text
90 分钟 Core Window
5 分钟左右重叠
Chunk Local Speaker → Episode Global Speaker
Core Window 决定最终文本归属
```

阈值、时间坐标、边界归属和低置信度策略现集中记录在 [`transcription.md`](transcription.md)，并由领域测试锁定。调整原因、影响和已知上限也在该文件中显式记录。

## Migration v4

`000004_transcription` 新增：

```text
transcription_runs
transcription_chunks
transcription_events
transcript_versions
transcript_speakers
transcript_segments
```

关键数据库不变量：

- 同一 Episode 同时最多一个非终态 Transcription Run。
- 同一 Episode 同时最多一个 active Transcript Version。
- Chunk sequence 在 Run 内唯一；Run 的 completed chunk 数不能超过总数。
- Run / Chunk 状态与 `completed_at` 保持一致。
- Transcript Version 内容不可被下一次转录覆盖。
- Segment 必须引用其来源 Chunk，Speaker 必须属于同一 Version。

所有访问使用 `pgx + sqlc`；状态推进、下一 Job 入队和持久事件写入同一 PostgreSQL 事务。

## Provider 与信任边界

业务工作流只依赖 `ASRProvider`、`ObjectStore`、`AudioDownloader` 和 `AudioProcessor` 契约，不直接引用阿里云 SDK 类型。

### Audio

- 源 URL 使用共享的 SSRF-safe HTTP Client：拒绝本机、私网、链路本地和 CGNAT，DNS 解析后再校验，并逐次校验 Redirect。
- 下载上限 2 GiB，限制响应头、总时长和支持的容器类型。
- FFmpeg 输出 16 kHz、mono、FLAC；FFprobe 的实际时长是切片唯一依据。
- 临时文件只存在于 Worker 本机，完成后删除。

### Aliyun ASR

```text
economy → paraformer-v2
quality → fun-asr
```

提交使用 DashScope 异步 HTTP API；每个 Poll Job 只请求一次，未完成时把同一个 PostgreSQL Job 延迟重新入队，不占用 Worker，也不消耗失败 attempt。生产 HTTP Client 同样阻断非公网目标，API Key 不进入业务数据或日志。

阿里云异步结果只保留有限时间。403/404 结果地址映射为稳定错误 `ASR_RESULT_EXPIRED`；自动流程不会重新计费，只有用户显式 Retry 才重新提交。

实现依据：

- [非实时语音识别](https://www.alibabacloud.com/help/en/model-studio/non-realtime-speech-recognition-user-guide)
- [Fun-ASR 录音文件识别 HTTP API](https://www.alibabacloud.com/help/zh/model-studio/fun-asr-recorded-speech-recognition-http-api)
- [语音识别模型与音频规格](https://www.alibabacloud.com/help/en/model-studio/asr-model/)
- [异步任务查询与取消](https://www.alibabacloud.com/help/en/model-studio/manage-asynchronous-tasks)

### Aliyun OSS

OSS 使用官方 Go SDK v2。对象 Key 由服务端生成并校验；自定义 Endpoint 必须是无凭据 HTTPS URL。源音频、标准化音频和 Chunk 只作为可恢复的临时对象，具体见转录算法文档。

SDK 依据：[OSS Go SDK v2 迁移指南](https://www.alibabacloud.com/help/en/oss/developer-reference/migration-guide-in-go)。

## Job 状态机与恢复

```text
download_audio
→ prepare_audio
→ plan_transcription
→ render_audio_chunk × N
→ submit_asr × N
→ poll_asr
→ ingest_asr_result × N
→ align_speakers
→ merge_transcript
→ cleanup_audio
```

重复 Job 先检查 Run / Chunk 状态，不重复推进。云端恢复点按以下优先级处理：

```text
已有 result_url       → ingest_asr_result
已有 external_task_id → poll_asr
已有 Chunk object     → submit_asr
没有 Chunk object     → render_audio_chunk
```

提交请求发生“服务端可能已收到、客户端未收到响应”的歧义时，Chunk 标记 `ASR_SUBMISSION_AMBIGUOUS`，不自动重提。明确失败或结果过期会清除失效云端引用；再次产生费用仍要求用户显式 Retry。

若失败 Run 之后已有更新的 active Run，旧 Run Retry 返回 409，避免数据库唯一约束泄漏为 500。

## API 与事件

OpenAPI 0.5.0 新增：

```text
POST /episodes/{episodeId}/transcriptions
GET  /transcriptions/{runId}
GET  /transcriptions/{runId}/events
POST /transcriptions/{runId}/retry
POST /transcriptions/{runId}/cancel
GET  /episodes/{episodeId}/transcript
GET  /transcripts/{transcriptId}/segments
PATCH /transcripts/{transcriptId}/speakers/{speakerId}
POST /transcripts/{transcriptId}/speakers/merge
```

事件先写 `transcription_events` 再通过 SSE 输出，因此断线后可使用 `Last-Event-ID` 继续读取。SSE 不是进度状态的唯一存储；客户端也可随时 GET Run。

新建与重试需要 Provider 配置，缺失时返回 503；读取已有 Transcript 不依赖云端 Provider。API 和 Worker 仍是同一模块化单体的两个进程。

## Transcript 与 Speaker

- ASR Local Speaker ID 不直接暴露为跨 Chunk 身份。
- Alignment 使用相邻 Chunk 的共同 Render 区间并建立一对一 Global Speaker 映射。
- 最终 Segment 只由句子中点所在 Core 拥有，边界不重不漏。
- 新 Version 在同一事务完整写入并取代 active 标记；旧 Version 保留且仍可按 ID 读取 Segment。
- Speaker 重命名保留 stable key；合并在事务中移动 Segment 后删除 source Speaker，并按 UUID 固定加锁顺序避免反向并发死锁。

## 对象生命周期调整

OSS 对象按最后使用点清理：

```text
预处理完成             → 删除源音频
全部 Chunk 渲染完成    → 删除预处理音频
单 Chunk 结果成功入库  → 删除该 Chunk 音频
completed / canceled   → 再次执行幂等兜底清理
failed                 → 只保留当前恢复点所需对象
```

原始 ASR JSON 不再写入 OSS，只长期保存标准化 Transcript。删除 Episode 会先取消对应转录 Job，收集所有对象 Key，在删除数据库记录的同一事务中创建独立 cleanup Job。已完成 Transcript、Version、Speaker 和 Segment 的级联删除已由真实 PostgreSQL HTTP 集成测试覆盖。

## 验收

本阶段的自动验收覆盖：

- 真正执行 FFmpeg 的 16 kHz FLAC 预处理与 Chunk 渲染。
- 阿里云异步提交、轮询、结果解析、取消和错误分类的 HTTP 合约测试。
- OSS Provider 的配置、Key 与 Signed URL 边界。
- PostgreSQL Job 延迟重新入队且不消耗 attempt。
- 3 小时音频走完整 API + Queue + Worker + Fake Provider 链路。
- 两个 90 分钟 Core 的 Local Speaker ID 对调后仍映射到相同 Global Speaker。
- 跨边界句子最终只出现一次。
- 已完成 Chunk 不重做；失败 Chunk 分别从 render / submit / poll / ingest 恢复。
- 第二次转录生成 Version 2，Version 1 保留且取消 active。
- Speaker 重命名、合并、SSE、取消和含 Transcript 的 Episode 删除。
- `go generate ./...`、全量 `go test ./...`、`go vet ./...`、Web build 与 `git diff --check`。

本地验收只使用独立 `echonote` PostgreSQL 数据库，没有使用 Docker，也没有对既有 `autoup` 数据库执行 Migration。由于仓库未提供真实阿里云密钥，本阶段没有产生云端 ASR / OSS 调用或费用；真实 Provider 合约由官方文档和本地 HTTP 测试覆盖，部署前仍需用目标账号做一条受控短音频 smoke test。
