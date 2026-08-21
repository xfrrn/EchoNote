# Migration Runbook

## 规则

- Migration 使用独立 `echonote_migrate` Role 和 `migration.env`，不在 API 或 Worker 启动时执行。
- Production 只执行 `up`；常规回滚不执行 Down Migration。
- 每次 Production Migration 前必须有已验证完成的数据库备份 ID。
- Migration 失败或 dirty 时停止发布，不手工修改 `schema_migrations`。

## 从 0 验证

在由测试 Migration Role 持有的隔离 `echonote_migration_check` 数据库安装 `pg_trgm`、`vector` 并应用 `runtime-grants.sql`。将 `EXPECTED_DATABASE_NAME` 和 Migration URL 指向该库，然后运行：

```bash
/opt/echonote/current/bin/echonote-migrate up
/opt/echonote/current/bin/echonote-migrate version
```

要求版本等于仓库最新 Migration，`dirty=false`。随后使用独立测试数据库执行 `go test -p 1 ./...`；包级串行避免多个集成测试包争用同一任务队列。不得连接 Production、Staging 或 `autoup`。

## Production Up

1. 记录 Release、当前 Migration version 和备份 ID。
2. 确认 API/Worker 仍运行旧兼容版本。
3. 切换 `current` 到新 Release，但暂不重启进程。
4. `systemctl start echonote-migrate.service`。
5. 读取 unit 日志并确认退出码为 0。
6. 以 Migration Role 重跑 `runtime-grants.sql`、`runtime-table-grants.sql` 和 `maintenance-grants.sql`。
7. 重启 API/Worker；两者启动时会拒绝超级用户、错误数据库名、Schema CREATE 权限或缺失扩展。

若 Migration 失败，恢复旧 symlink，保留当前数据库和失败证据。只有在独立恢复演练中才允许使用 `migrate down`; Production 禁止直接执行。
