# 备份与隔离恢复 Runbook

## 目标与责任

- RPO 24 小时，RTO 4 小时。
- PostgreSQL 使用托管平台的加密每日备份，保留 30 天；平台支持时同时启用至少 7 天 PITR。
- OSS Production Bucket 启用服务端加密和至少 7 天版本/回收保护。生命周期规则不得先于 Episode 删除清除 raw ASR 或音频对象。
- 备份服务使用独立凭据和独立故障域，API、Worker、Migration Key 无权删除备份。账号所有者和数据库值班各保留一个恢复入口。

选择托管数据库原生备份，不在应用主机自制 `pg_dump` 定时器：它已提供加密、保留、PITR、跨故障域和任务告警，减少一套无人维护的备份程序。平台若缺少任一能力，首次上线前迁移到满足能力的平台或补充经过恢复演练的外部备份产品，不能只把文件留在同一主机。

## 上线前配置

1. 对 Production 数据库启用每日备份，窗口避开预期高峰，保留 30 天；记录策略 ID 和最近成功 Backup ID。
2. 启用 7 天以上 PITR，并验证数据库时区、WAL/日志保留与加密状态。
3. 为备份失败、26 小时无成功备份和存储不可达配置 `alerts.md` 中的 page。
4. OSS 启用版本/回收保护和服务端加密；用独立测试对象验证删除后 7 天内可恢复。
5. 发布记录必须包含 Backup ID、策略截图/导出、告警测试和恢复演练日期，不保存数据库密码或对象内容。

## 每季度隔离恢复

1. 选择最近每日备份；另一次年度演练选择 PITR 时间点。记录 Backup ID、时间和预期 RPO。
2. 恢复到新实例和新数据库 `echonote_restore_<yyyymmdd>`。安全组只允许演练主机，禁止 Production API/Worker 网络访问；不要恢复或复用 Production Provider Key、Cookie Key 或 OSS 写 Key。
3. 用只属于恢复实例的 libpq service 配置连接。确认当前数据库名后才运行任何命令，绝不连接 `autoup`、Staging 或 Production。
4. 从对应 Release 执行只读校验：

```bash
export PGSERVICE=echonote-quarterly-restore
export RESTORE_EXPECTED_DATABASE=echonote_restore_20260821
export EXPECTED_MIGRATION_VERSION=8
/opt/echonote/current/ops/scripts/verify-restore.sh
```

5. 在隔离主机用恢复专用 API/Worker Role 启动同版本二进制，不运行 Migration。登录演练用户，读取 Episode、Note、Transcript/Speaker、Keyword/Hybrid Search、AI Artifact/Citation 和 Export；记录随机抽样 ID 与成功/失败，不复制正文。
6. 审核脚本输出的 queued/running/failed Job。queued 可在隔离 Worker 使用 Fake/禁网 Provider 验证调度；running Job 必须先按事故决定回收或取消，绝不让隔离恢复调用真实 Provider。
7. 运行 `/opt/echonote/current/ops/scripts/smoke.sh`（隔离 HTTPS 域名）并验证 `/readyz`、Session 和安全头。测量从开始恢复到业务读取成功的时间，必须小于 4 小时。
8. 记录 Migration `dirty=false`、关系计数、孤儿数为 0、业务读取、实际 RPO/RTO 和发现的问题。证据审批后按平台回收流程删除隔离实例及临时凭据，并确认删除目标确为 `echonote_restore_*`。

恢复演练失败、超过 RTO、备份不可解密/不可用或业务关系异常都阻塞发布；修复后必须重做，不接受“备份任务显示成功”替代恢复证据。
