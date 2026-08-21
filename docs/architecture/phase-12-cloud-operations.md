# Phase 12：云端、监控与灾备实施记录

- 状态：仓库内实现与本地验收完成；真实 Staging Provider、托管备份恢复、Linux 告警投递和 iOS Safari 证据待执行
- 日期：2026-08-21

## 当前代码 → 目标

Phase 11 之后已有原生部署，但没有可执行数据留存、Provider 成本日志、告警阈值、恢复校验或统一 Release Gate。目标是在不新增监控集群和自制备份系统的前提下，把 Production 必需操作变成受限命令、只读检查和可审计 Runbook，并明确外部 Gate 不能由本地结果替代。

## 修改文件

- Migration 8、Import Query：历史 Job 可按策略删除，Import 保留并回退为稳定 succeeded/failed 状态。
- `cmd/maintenance`、`repository/maintenance`：系统范围 Retention 先 Dry Run；只有显式 `--apply-system` 删除 30 天 Session、30 天 succeeded/canceled Job 和 90 天 failed Job，Event 级联删除。
- Maintenance systemd Timer 与列级 PostgreSQL Grant：默认不启用；Role 不可读 Session Token Hash。Worker Role 完全隔离 `users/sessions`，对 Note 只读。
- Worker/Provider 日志：心跳、Job 关联 ID、耗时、稳定错误码、Provider 状态、ASR 音频时长、Embedding 数量和 LLM Token；测试保证不记录 Prompt、Transcript、回答、URL、对象 Key 或 Payload。
- Embedding/LLM Provider：网络、408、429、5xx 有限重试；认证与确定性 4xx 不重试。同步 AI Provider 不可用返回 503。
- `deployments/monitoring`：平台原生告警阈值、Queue/失败 Job/连接/留存/每日用量只读 SQL。
- `backup-restore.md`、`verify-restore.sh`：托管加密备份/PITR/OSS 回收保护和季度隔离恢复；脚本强制 `echonote_restore_*`，验证 Migration、扩展、FK、孤儿关系、业务表计数与未完成 Job。
- `provider-smoke.md`、`release-gate.md`：真实云矩阵、无内容证据格式和阻塞式发布清单。
- `browser-ci-smoke.sh`、CI：只允许一次性 EchoNote 测试库，自动启动 Production Build 与 API，使用真实 Chrome 覆盖登录、三类 Import 请求、Transcript/SSE/Speaker、Note、三类 Keyword Search、AI/Citation SSE、四种 Export/Clipboard、离线 Outbox、夹具清理、退出和 401。

## 本地验收证据

- 全新隔离 PostgreSQL 17 + pgvector 从 0 到 Migration 8，`dirty=false`；Migration 8 Down 到 7 后 Up 回 8 通过。
- `verify-restore.sh` 在 `echonote_restore_local` 实跑通过，关系孤儿为 0；脚本和 HTTPS Smoke 均通过 `bash -n`。
- Maintenance Role 实跑 Dry Run/Apply；Session `expires_at/revoked_at` 可读、`token_hash` 不可读；Worker 对 `users/sessions` 无权限、对 Note 只读。
- Ubuntu 24.04 WSL 的 Retention Service Dry Run/Apply 均成功，Timer 已启用并产生下一次调度；健康 SQL 以 API Role 实跑通过。Job Queue 集成测试覆盖 stale Lease 回收、replacement worker attempt 2 和最终完成事件。
- 独立测试库 `go test -p 1 ./... -count=1`、`go vet ./...`、`pnpm build` 通过；Go/TypeScript 生成物二次 SHA-256 一致。
- Nginx 1.24 `nginx -t`、健康 SQL、Node Theme 脚本和 `git diff --check` 通过；systemd 255 可解析并在 Ubuntu WSL 实际运行新增 Service/Timer。
- 真实桌面 Chrome 加载 Production Build：用户名/密码登录、刷新保留 Session、离线 Capture 刷新后仍在 IndexedDB、联网后服务端恰好一条 Note、退出后 Cookie 清除。Manifest 无 installability error，Service Worker activated；通过 Chrome PWA 协议实际安装并启动独立窗口，`display-mode: standalone`，离线刷新仍显示应用壳；“复制全文”状态成功且剪贴板包含节目标题和笔记。localhost 安全上下文仅用于避开本地自签证书的应用窗口限制，不替代真实证书、OS Share Sheet、真实 Provider 或 iOS 证据。
- 可重复的 Playwright CLI Browser Smoke 在一次性 PostgreSQL 测试库连续实跑通过：输出 `imports:3`、可恢复转录 SSE、Speaker 合并后 1 人、Note/Transcript/AI 三类 Keyword Search、AI/Citation SSE、`exports:4`、`offlineOutbox:true`、`logoutStatus:401`；选段 Export 实际翻到第 2 页选择第 101 段，全文 Export 同时包含首段与第 101 段。API 日志对应 3 次 Import、4 次 Export、1 次幂等 Capture、2 次 Speaker 修改且无非预期 4xx/5xx。脚本拒绝非 `echonote_test*`/`echonote_browser_*` 数据库并允许显式无冲突端口。
- Browser Smoke 的 Import 完成态、AI 回答流和已完成 Transcript/Artifact 使用 Test-only 确定性夹具；其余浏览器路径走真实 API。该边界验证 Web 闭环，不替代 Staging 的真实解析、ASR、Embedding/Hybrid、LLM 或 Provider SSE 证据。

## 外部阻塞 Gate

- 按 `provider-smoke.md` 使用独立限额云账号完成 Apple/RSS/Direct、两档 ASR、双 Speaker 长音频、OSS、Hybrid、Qwen/SSE/Citation 和四种 Export。
- 在目标平台配置每日 30 天备份、至少 7 天 PITR/OSS 回收保护并完成含真实业务关系的季度隔离恢复，实际 RPO <=24h、RTO <=4h。
- 全新 Ubuntu 主机完成 HTTPS、重启、Lease、SSE、权限和全部告警投递；由另一位工程师按 Runbook 复现。
- 当前桌面 Chrome 补做 OS Share Sheet 和真实 Provider 主路径；真实 iOS Safari 完成安装、刷新、离线 Outbox、Clipboard/Share Sheet。

上述任一项无证据时，`release-gate.md` 必须保持 PENDING/BLOCKED，项目不能标记为 Production-ready。
