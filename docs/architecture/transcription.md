# EchoNote 转录切片与 Speaker 对齐设计

更新时间：2026-08-20

## 文档来源与调整说明

整体方案 v1.0 已引用本文件，但仓库、Git 历史和远端均未包含该文件。因此本文件不是替换已有算法，而是把整体方案中已经确定的原则补成可实现、可测试的参数：

```text
90 分钟 Core Window
+ 左右重叠
+ Chunk Local Speaker → Episode Global Speaker
+ Core Window 决定文本归属
```

固定参数如需调整，必须先修改本文件和相应测试，不能只改 Worker 常量。

## 输入与档位

```text
economy → aliyun / paraformer-v2
quality → aliyun / fun-asr
```

两档都开启 `diarization_enabled`。Paraformer 同时开启时间戳对齐，确保句子和词都有毫秒时间戳。阿里云离线 ASR 接受 FLAC，且开启说话人分离时建议单个音频不超过 2 小时；EchoNote 因此统一预处理为 16 kHz、单声道 FLAC。

官方契约：

- [阿里云非实时语音识别](https://www.alibabacloud.com/help/en/model-studio/non-realtime-speech-recognition-user-guide)
- [Fun-ASR 录音文件识别 HTTP API](https://www.alibabacloud.com/help/zh/model-studio/fun-asr-recorded-speech-recognition-http-api)
- [语音识别模型与音频规格](https://www.alibabacloud.com/help/en/model-studio/asr-model/)

## 音频预处理

流程：

```text
安全下载源音频
→ SHA-256
→ FFmpeg 解码
→ 16 kHz / mono / FLAC
→ FFprobe 获取实际 duration_ms
→ 上传对象存储
```

Episode Resolver 提供的时长只用于展示。切片必须使用预处理文件的 FFprobe 时长，避免元数据误差造成尾部遗漏。

## 切片

固定参数：

```text
core_window = 90 分钟
overlap = 5 分钟
```

第 `i` 个 Chunk：

```text
core_start = i * core_window
core_end   = min(duration, core_start + core_window)

render_start = max(0, core_start - overlap)
render_end   = min(duration, core_end + overlap)
```

所有区间均为毫秒、左闭右开 `[start, end)`。中间 Chunk 最长 100 分钟，低于开启说话人分离时建议的 2 小时上限。短于或等于 90 分钟的音频只有一个 Chunk，不额外增加重叠。

每个 Chunk 的云端任务指纹为：

```text
SHA-256(
  chunk_audio_hash
  + render_start_ms
  + render_end_ms
  + provider
  + model
  + canonical_config_json
)
```

同一 Chunk 重试时复用已保存的 `external_task_id`。如果提交请求可能已经到达云端但客户端没有收到响应，自动重试停止并标记 `ASR_SUBMISSION_AMBIGUOUS`；只有用户显式调用 Retry 才允许再次产生费用。

轮询或结果下载的临时网络错误保留 `external_task_id` / `result_url`，显式 Retry 从原位置继续。阿里云任务和结果链接有有效期；结果地址明确返回 403/404 时记录 `ASR_RESULT_EXPIRED` 并清除失效云端引用，但不自动重提。用户再次 Retry 后才从 `submit_asr` 重新开始。

## 时间坐标

ASR 返回 Chunk 内局部时间。标准化后先换算到整期 Episode：

```text
episode_time = render_start_ms + local_time
```

无效句子（空文本、负时间或 `end <= start`）丢弃；超出音频时长的时间戳裁剪到 `[0, duration_ms]`。只把标准化结果写入 PostgreSQL，不把原始 ASR JSON 长期保存到 OSS。

## Local Speaker 对齐

阿里云 `speaker_id` 只在单个 Chunk 内有效。EchoNote 按 Chunk 顺序建立 Global Speaker：

1. 第一个 Chunk 按 Local Speaker 首次出现时间创建 `speaker-001`、`speaker-002`……。
2. 后续 Chunk 只在它与前一个 Chunk 的共同 Render 区间比较。由于两边各扩展 5 分钟，中间边界的共同区间为 10 分钟。
3. 对当前 Local Speaker `L` 与已有 Global Speaker `G`，累计两边句子在同一 Episode 时间轴上的交集时长：

```text
matched_ms(L,G) = Σ intersection_duration(current_L, previous_G)
evidence_ms(L)  = Σ current_L 在共同 Render 区间内的时长
confidence      = min(1, matched_ms / evidence_ms)
```

4. 候选按 `matched_ms DESC, confidence DESC, local_id, global_key` 排序，贪心建立一对一映射。
5. 只有 `matched_ms >= 2000` 且 `confidence >= 0.50` 才复用 Global Speaker；其余 Local Speaker 创建新的 Global Speaker，并记录低置信度事件。

一对一约束可正确处理相邻 Chunk 的 Local ID 对调。重叠区没有发言证据时不猜测身份；第一版不做跨节目声纹识别，也不根据姓名文本推断人物。

当前贪心匹配的已知上限是多人同时说话且候选分数接近的密集场景。如果生产指标显示低置信度或误配集中在该场景，再以最大权重二分图匹配替换，输入和阈值保持不变。

## Transcript 合并与归属

对每个标准化句子计算中点：

```text
midpoint = start_ms + (end_ms - start_ms) / 2
```

只有中点落在该 Chunk Core `[core_start, core_end)` 的句子才进入最终 Transcript。重叠区只用于 Speaker 对齐，不拥有文本。这样每个时间点只属于一个 Core，不需要基于易变化的 ASR 文本做跨 Chunk 字符串去重。

最终 Segment 按 `start_ms, end_ms, chunk_sequence, local_sequence` 稳定排序并重新生成连续 `sequence`。Segment 保留完整句子和词时间戳，不为边界强行截断文字。

## Transcript Version

每次重新转录创建新 Version：

```text
生成完整 Version
→ 同一事务取消旧 is_active
→ 激活新 Version
→ 保留旧 Version
```

任何时候同一 Episode 最多一个 `is_active=true`。ASR 生成的 Version 内容不被下一次转录覆盖。用户重命名与合并 Speaker 属于当前 Version 的人工校正元数据；合并会更新该 Version 的 Segment Speaker。Phase 6 已在同一事务创建 Search 重建 Job；Phase 7 已在同一事务把关联 AI Artifact 标记为 stale 并取消未完成的生成 Job。

## Job 工作流

```text
download_audio
→ prepare_audio
→ plan_transcription
→ render_audio_chunk × N
→ submit_asr × N
→ poll_asr（每次最多一次 HTTP 查询，未完成则延迟 5 秒重新入队）
→ ingest_asr_result × N
→ align_speakers
→ merge_transcript
→ cleanup_audio
```

每个状态推进与下一 Job 入队放在同一 PostgreSQL 事务。重复 Job 通过 Run/Chunk 当前状态成为 no-op。单个失败 Chunk 显式 Retry 时只重做该 Chunk；已经完成的 Chunk 不重复提交。

## 对象生命周期

路径：

```text
users/{user_id}/episodes/{episode_id}/transcription-runs/{run_id}/source/original
users/{user_id}/episodes/{episode_id}/transcription-runs/{run_id}/source/prepared.flac
users/{user_id}/episodes/{episode_id}/transcription-runs/{run_id}/chunks/{sequence}.flac
```

- 原始音频：预处理 FLAC 写入 OSS 并记录成功后立即删除。
- 预处理 FLAC：全部 Chunk 写入 OSS 并记录成功后立即删除。
- Chunk 音频：对应 ASR 结果完成读取并标准化入库后立即删除。
- Run 完成或取消时再次扫描已记录的对象 Key，执行幂等兜底清理；删除失败由 Cleanup Job 重试。
- 失败步骤只保留从当前恢复点继续所必需的对象；Retry、Cancel 或删除 Episode 会继续处理或清理。

Bucket 仍应设置兜底生命周期，处理进程在对象写入后、数据库记录前异常退出等无法由应用追踪的孤儿对象；生命周期时长同时也是失败任务可恢复窗口。
