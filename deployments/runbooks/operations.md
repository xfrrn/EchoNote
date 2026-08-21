# 运行维护 Runbook

## 临时文件与对象

- Worker 的唯一可写目录是 `/var/lib/echonote/tmp`。启动时只删除超过 `WORKER_TEMP_MAX_AGE` 的 `echonote-*` 常规文件，不遍历目录或符号链接。
- 原始音频在预处理完成后删除，预处理 FLAC 在全部 Chunk 生成后删除，每个 Chunk 在结果入库后删除；`completed` / `canceled` 再做兜底清理，Failed 只保留当前恢复点所需对象。
- 私有 Bucket 必须配置兜底生命周期，用于清理由进程崩溃产生的未记录孤儿对象；生命周期时长也是失败任务最长恢复窗口。
- 主机临时目录和私有 Bucket 使用量达到 80% 时告警。若单个合法任务可能运行超过 24 小时，先提高 `WORKER_TEMP_MAX_AGE`。

## Cleanup Job 失败

告警必须包含 `job_id`、`user_id`、`entity_id` 和 `error_code`。确认对象 Key 属于 `users/<user-id>/episodes/...` 前缀后，用受限 oneshot 重试：

```bash
sudo systemctl start "echonote-admin-retry-cleanup@<job-id>.service"
sudo journalctl -u "echonote-admin-retry-cleanup@<job-id>.service" -n 50 --no-pager
```

命令使用无 Provider 密钥、无 Schema 权限的 `echonote_maintenance` Role，只接受 `status=failed AND type=cleanup_audio` 的 Job，并创建 `manual_retry` Event；其他 Job 会失败退出。

## Lease 恢复

Worker 每个进程一次只执行一个 Job，并按 Lease 的三分之一周期续租。Worker 异常终止后，其他 Worker 在 `WORKER_LEASE_TIMEOUT` 后重排 Job。检查 `JOB_LEASE_EXPIRED` Event、Provider task ID 和最终实体唯一约束，不能直接复制 Job。

## 数据留存

首次只运行系统范围 Dry Run，并把四个计数附到变更记录：

```bash
sudo systemd-run --quiet --wait --pipe --collect --uid=echonote-maintenance \
  -p EnvironmentFile=/etc/echonote/common.env \
  -p EnvironmentFile=/etc/echonote/maintenance.env \
  /opt/echonote/current/bin/echonote-maintenance retention --dry-run
```

核对过期或撤销 Session 为 30 天、succeeded/canceled Job 为 30 天、failed Job 为 90 天；Job Event 随 Job 级联删除。统计获批后执行一次 `sudo systemctl start echonote-retention.service`，复核日志和业务读取，再用 `sudo systemctl enable --now echonote-retention.timer` 启用每日任务。命令没有普通 `--apply`，只有明确的 `--apply-system`；用户数据内容、Embedding、Transcript、Note 与 AI Artifact 不在此任务内删除。

## 证书与密钥

- 每月运行 `certbot renew --dry-run`；证书剩余 21 天时告警。
- 密钥轮换：更新对应 root-only env 文件，重启单个服务，运行 Smoke，再撤销旧 Key。
- Staging 与 Production 的数据库、Bucket、Role 和 Provider Key 不得复用。
