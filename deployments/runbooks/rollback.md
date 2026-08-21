# Rollback Runbook

## Web / 二进制回滚

1. 停止继续发布，记录失败 Release、请求 ID、Migration version 和备份 ID。
2. 确认上一 Release 目录仍存在且其后端兼容当前加法 Schema。
3. 原子把 `/opt/echonote/current` 指回上一 Release。
4. 执行 `nginx -t` 和两个 `--check-config` 预检。
5. 重启 API、Worker，重载 Nginx，运行 HTTPS Smoke。
6. 不执行 Down Migration；保留新表和新列。

静态 Web 与二进制位于同一不可变 Release，可一起回滚。若只需回滚 Web，复制上一 Release 的 `web/` 到一个新的不可变 Release 并切换 symlink，不能覆盖历史目录。

## 数据库恢复（仅灾难路径）

只有 Schema 已与旧程序不兼容、加法回滚无法恢复服务，并且对应恢复演练已通过时才进入本路径：

1. 保持现有数据库只读并保留取证，不在原库上覆盖恢复。
2. 将发布前备份恢复到新的独立 PostgreSQL 实例。
3. 验证 Migration version/dirty、数量、随机 Episode、Note、Transcript、AI Citation 和登录读取。
4. 更新 API/Worker 密钥文件指向恢复实例，先在内部完成 readiness 和 Smoke。
5. 明确批准后切流；记录实际 RPO/RTO。

绝不把恢复实例连到 `autoup` 或其他业务数据库，也不使用 Production Down Migration 代替恢复。
