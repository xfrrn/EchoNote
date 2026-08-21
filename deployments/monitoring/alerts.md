# 生产监控与告警基线

首版使用托管平台的日志、主机、PostgreSQL、Provider 和证书监控；不要仅为 EchoNote 部署 Prometheus。所有告警必须有 Production/Staging 标签、负责人、Runbook 链接和最近成功时间。

## 结构化日志

- API 请求日志：`request_id`、`user_id`、`method`、`path`、`status`、`duration_ms`。
- Worker Job 日志：`worker_id`、`user_id`、`job_id`、`episode_id` 或 `transcription_run_id`、`operation`、`provider`、`duration_ms`、`error_code`。
- API/Worker Provider 日志继承关联 ID，并含 `provider`、`operation`、`duration_ms`、`status`；失败含 `error_code`/`provider_status`，ASR Submit 含 `audio_duration_ms`，LLM 成功含 `input_tokens`/`output_tokens`，Embedding 含 `input_count`。
- AI Token 和 ASR/Embedding 用量以数据库中的持久化 Usage 汇总，不把 Prompt、Transcript、Note、音频 URL、对象 Key、Cookie、Authorization 或 Job Payload 写入日志。
- 日志保留 30 天；只允许值班和安全人员读取，导出同样受 30 天删除策略约束。

## 必须配置的告警

| 信号 | 触发条件 | 严重度 | 处理入口 |
| --- | --- | --- | --- |
| API 5xx | 5 分钟至少 20 个请求且 5xx 比例 > 5% | page | 请求日志按 `request_id` 关联；检查 `/readyz` |
| Readiness | 主机本地探针连续 3 次 `http://127.0.0.1:8080/readyz` 非 200 | page | `deploy.md` 与 PostgreSQL 状态 |
| Worker 活性 | 模板配置下 4 分钟无 `worker heartbeat` 且无 Job started/completed/rescheduled/failed；修改 Lease 后阈值取 `max(4m, 2/3 × WORKER_LEASE_TIMEOUT)` | page | `operations.md#lease-恢复` |
| Queue Delay | `resolve_episode` > 2 分钟；转录/Embedding/AI > 10 分钟；cleanup > 15 分钟 | ticket；超过 2 倍 page | `health.sql` 与 Worker 日志 |
| Failed Job | 5 分钟出现任何新 failed Job | ticket | 按 `error_code` 聚合 |
| Cleanup 失败 | 任一 `cleanup_audio` failed | page | `operations.md#cleanup-job-失败` |
| PostgreSQL 连接 | 连接数 > `max_connections` 的 80%，持续 5 分钟 | page | 连接预算与长事务 |
| PostgreSQL CPU/磁盘 | 任一 > 80%，持续 15 分钟；磁盘预测 7 天内满也告警 | page | 托管数据库面板 |
| Worker 临时目录 | `/var/lib/echonote/tmp` 所在文件系统 > 80%，持续 15 分钟 | page | `operations.md#临时文件与对象` |
| OSS 用量 | Bucket 容量或账号配额 > 80%；无法设置硬配额时按日增长预测 7 天内越过预算 | ticket；对象写入受阻时 page | 生命周期与 cleanup Job |
| Provider 认证 | 任一 401/403 | page | 轮换/权限，不自动无限重试 |
| Provider 限流 | 5 分钟 >= 3 个 429 | ticket；任务停滞则 page | 限额、并发和重试 |
| Provider 5xx | 5 分钟 >= 5 个 | ticket；持续 15 分钟 page | Provider 状态页与请求 ID |
| Provider 预算 | 当日 ASR、Embedding 或 LLM 用量达到预算 80%/100% | ticket/page | Provider 面板与 `health.sql` |
| TLS | 证书剩余 <= 21 天；<= 7 天升级 page | ticket/page | `certbot renew --dry-run` |
| 备份 | 每日备份失败或 26 小时无成功备份 | page | `backup-restore.md` |
| 留存 | `echonote-retention.service` 失败或 Timer 36 小时未成功 | ticket | Dry Run 与 Maintenance 日志 |

上线前把实际 Provider 日预算、告警通知目标和主/备负责人写入部署平台；仓库不保存账号、手机号或 Key。每季度恢复演练时同时测试一次告警投递，避免只有规则没有接收人。
