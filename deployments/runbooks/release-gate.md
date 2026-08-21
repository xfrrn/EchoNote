# Production Release Gate

发布负责人逐项附证据并签名。`BLOCKED`、`SKIPPED`、缺失证据和“仅代码审查”都不是 PASS。真实 Linux、云 Provider、恢复演练和 iOS Safari 无法由本地测试替代。

| Gate | 必需证据 | 状态 |
| --- | --- | --- |
| 从 0 Migration | 全新独立库 `migrate up` 到版本 8，`dirty=false`；扩展和 FK 有效 | PENDING |
| 自动化质量 | `go generate`/Contract 无差异，`go test -p 1 ./...`、`go vet ./...`、`pnpm build`、`browser-ci-smoke.sh` | PENDING |
| 浏览器核心流程 | 登录、三类导入、转录/Speaker、Note、Search、AI/SSE、四种 Export、离线 Outbox | PENDING |
| 身份与隔离 | 无固定生产用户；Cookie/CSRF/跨用户/SSRF/登录限速测试和实际 Header | PENDING |
| 密钥与权限 | Git/日志/Web Bundle 无 Key；API/Worker/Maintenance/Backup Role 与 Bucket 最小权限 | PENDING |
| 原生运行 | 全新 Ubuntu 按 Runbook 部署；API/Worker/主机重启、Lease 回收、SSE 重连、TLS 续期 | PENDING |
| 真实 Provider | `provider-smoke.md` 全矩阵，长音频在规定触发条件下有有效证据 | PENDING |
| 删除与留存 | Episode 对象 cleanup、失败告警/重试；Retention Dry Run、一次受控 Apply、Timer | PENDING |
| 告警 | API、readyz、Worker、Queue、Job、PG、Provider、预算、TLS、Backup 告警投递 | PENDING |
| 备份与恢复 | 最近 Backup ID；隔离恢复、关系/业务读取、RPO <=24h、RTO <=4h | PENDING |
| 客户端 | 当前桌面 Chromium 与真实 iOS Safari PWA：安装、刷新、离线、Share Sheet | PENDING |
| 回滚 | 旧 Release Symlink 回切成功，不执行 Down Migration | PENDING |

## 自动化命令

```bash
corepack enable
pnpm install --frozen-lockfile
pnpm generate:contracts
(cd apps/server && go generate ./...)
git diff --exit-code
(cd apps/server && ECHONOTE_TEST_DATABASE_URL="$TEST_DATABASE_URL" go test -p 1 ./... -count=1)
(cd apps/server && go vet ./...)
pnpm build
npx --yes --package @playwright/cli@0.1.18 playwright-cli install-browser chrome --with-deps
DATABASE_URL="$TEST_DATABASE_URL" bash deployments/scripts/browser-ci-smoke.sh
git diff --check
```

`TEST_DATABASE_URL` 必须指向一次性独立 `echonote_test`，不能与 Development、Staging 或 Production 共用。Browser Smoke 输出必须包含 `imports:3`、`exports:4`、`offlineOutbox:true` 和 `logoutStatus:401`；本机端口冲突时只可用 `BROWSER_SMOKE_WEB_PORT`、`BROWSER_SMOKE_API_PORT` 改端口，不能跳过流程。

Browser Smoke 的 Test 环境边界是：测试夹具预置已完成 Transcript/AI Artifact；Apple/RSS/Direct 的创建请求进入真实 API，但浏览器替身只控制异步 Import 完成态；Conversation 由真实 API 创建，但回答 SSE 使用确定性浏览器替身。登录、Session、Library、Transcript 读取与 SSE 断点恢复、Speaker 修改、Keyword Search、四种 Export、Clipboard、IndexedDB 离线刷新/单次重放、删除、logout 均走真实 API。该结果只证明浏览器闭环和 Fake Provider 边界，不能替代 Staging 真实 Provider、Hybrid Search 或 OS Share Sheet 证据。

随后执行 Nginx `nginx -t`、systemd `systemd-analyze verify`、`smoke.sh`、`provider-smoke.md`、`backup-restore.md` 和真实客户端验收。发布记录包含 commit、Release ID、Migration version、Backup ID、Staging/Production 域名、证据链接、执行人和批准人；不包含任何 Secret 或用户正文。

Staging 首次验收以 `VERIFY_LOGIN_RATE_LIMIT=1 deployments/scripts/smoke.sh https://<staging-domain>` 运行登录限速探针；它在正常登录/退出完成后用无效测试身份确认 Nginx 返回 429，并会让执行 IP 短暂受限。常规发布 Smoke 不设置该变量。

只有全部行被证据替换为 PASS 后，才能给 Release 打 Production-ready 标记并放量。失败时保持旧 Release，按 `rollback.md` 回切；不得为了通过 Gate 降低安全检查、关闭告警或执行 Down Migration。
