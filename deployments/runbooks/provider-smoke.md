# Staging 真实 Provider Smoke

本 Runbook 只在独立 Staging 数据库、私有 Bucket 和限额 Key 上执行。测试媒体必须已获授权且不含 Production 用户内容。长音频只在首次上线、转录算法改变或 Provider 大版本改变时重跑。

## 证据格式

每个 Case 只记录：Git commit、时间、Staging 请求 ID/Provider 请求 ID、Provider、模型、`duration_ms` 或 `audio_duration_ms`、`input_tokens`/`output_tokens`、状态、费用单位和执行人。不得附 Key、Cookie、完整 URL、对象 Key、Prompt、Transcript、Note、AI 正文或音频。

| Case | Request ID | Provider / Model | Duration / Tokens | Status | Cost unit | 执行人/时间 |
| --- | --- | --- | --- | --- | --- | --- |
| Apple 单集 |  |  |  |  |  |  |
| RSS 最新单集 |  |  |  |  |  |  |
| Direct Audio |  |  |  |  |  |  |
| Economy 短音频 |  | Aliyun / paraformer-v2 |  |  |  |  |
| Quality 短音频 |  | Aliyun / fun-asr |  |  |  |  |
| 双 Speaker >90 分钟 |  |  |  |  |  |  |
| OSS Signed URL / cleanup |  | Aliyun OSS |  |  |  |  |
| Embedding / Hybrid |  | text-embedding-v4 |  |  |  |  |
| Qwen Artifact / Conversation |  | qwen-plus |  |  |  |  |
| 四种 Export |  |  |  |  |  |  |

## 预检

1. Release commit 与待发布 commit 完全一致；`APP_ENV=staging`，数据库名以 `echonote_` 开头，Bucket 和 Key 均为 Staging 专用。
2. Provider 账号设置日预算和 80%/100% 告警。Key 只拥有目标模型和单一私有 Bucket 所需权限。
3. `smoke.sh https://<staging-domain>` 通过；`/readyz` 仅本机可见；Nginx SSE 无缓冲。
4. 准备 Apple Podcasts 单集 URL、RSS Feed URL、可直接下载的短音频，以及一份超过 90 分钟且含两位清晰 Speaker 的音频。证据只使用内部 Case 名，不粘贴 URL。
5. 打开 API/Worker JSON 日志和 Provider 请求面板，确认可按 `request_id`、`job_id` 和 Provider Request ID 关联。

## 执行矩阵

1. 分别从 Web 导入 Apple 单集、RSS Feed 和 Direct Audio。三项都必须从 queued/running 到 succeeded，Episode 去重正确，页面刷新后仍存在；RSS 选择最新有效音频。
2. 对短音频先执行 `economy`，完成后执行 `quality`。分别确认模型为 `paraformer-v2`、`fun-asr`，Transcript Version 递增且只有一个 Active Version，时间戳单调、无空 Segment。
3. 对长音频以 `speaker_count=2` 执行一次。确认至少两个 Core Window/Chunk、两个可编辑 Speaker、跨 90 分钟边界无明显重复或缺口；刷新后 Speaker 合并/改名仍保持。
4. 在 OSS 控制台确认上传对象私有、Signed URL 有效且过期后拒绝。删除专用测试 Episode，等待 `cleanup_audio` succeeded，再确认该 Episode 前缀对象不可读；失败时按 Operations Runbook 重试并保留 Event 证据。
5. 等待 `text-embedding-v4` 完成。用 Transcript 中存在的词和同义表达各搜索一次；必须返回 `mode=hybrid`、目标 Episode 结果和合法 Citation 时间范围。Provider 不可用的降级 `mode=keyword` 另做故障演练，不能算真实 Provider 通过。
6. 请求 Episode AI Artifact。确认 ready 结果包含 Summary、Key Points、Speaker Views、Worth Reviewing 和合法 Note Connections；所有 Segment/Note ID 必须来自当前 Episode。记录 Artifact Token，不保存正文。
7. 创建 Conversation，通过 SSE 提问并收到 delta、citation、done。断线后重连/刷新，确认 Message 与 Citation 持久化；相同 Client Message ID 重放不得产生第二次计费请求。记录 Provider Request ID 和 Token。
8. 依次生成 `notes_only`、`organized_note`、`selected_transcript`、`full_transcript`。Web Clipboard 和系统 Share Sheet 至少各执行一次；证据只记录成功和输出字节数，不保存导出内容。
9. 查询 `/opt/echonote/current/ops/monitoring/health.sql` 的当日 ASR 秒数、Embedding Chunk 和 LLM Token，与 Provider 账单/用量面板核对；误差必须解释并低于已配置日预算。
10. 撤销本轮临时 Staging Key 或恢复到常规最小权限，删除测试数据前确认 cleanup Job 成功。

任何 Provider 401/403、无法解释的重复计费、Citation 越界、私有对象可公开访问、清理失败、两档模型错误或 SSE 断线不可恢复都阻塞发布。将填好的表、Provider 面板导出和日志查询保存到受控发布记录，不提交 Git。
