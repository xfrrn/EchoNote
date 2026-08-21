# Phase 10：Web 真实数据闭环实施记录

- 状态：仓库实现完成，真实 iOS Safari 验收待执行
- 日期：2026-08-21

## 当前代码 → 目标

业务页面原先直接读取 `shared/mock`，`packages/contracts` 也是占位。目标是让生产路由只通过同源 `/api/v1` 使用 Session API，并完成导入、记录、转录、搜索、AI、导出和离线记录闭环。

## 修改文件

- `packages/contracts/src/schema.ts`：由后端 OpenAPI 生成并提交；根目录只保留 `pnpm generate:contracts` 一个契约生成命令。
- `apps/web/src/shared/api`：统一 Cookie 请求、稳定错误解析、401 通知和 AI SSE 解析。
- `apps/web/src/features/auth`、业务页面与路由：接入登录、Library、Import、Notes、Transcription、Transcript、Search、AI 和 Export API；Mock 仅保留在 Design Playground。
- `apps/web/src/shared/outbox`：用 IndexedDB 先存后发，按时间重放；4xx 阻塞人工处理，网络和 5xx 有上限退避。
- `apps/web/vite.config.ts`：缓存版本化静态资源；导航回退不覆盖 `/api`，API、Session、SSE 和用户内容均不进入运行时缓存。
- `.github/workflows/ci.yml`：重新生成 Go、sqlc 和 TypeScript 契约后检查工作树无差异，并执行后端测试、vet 与 Web 构建。

## 风险与边界

- 离线范围只覆盖新建记录；编辑和删除保持联网操作。
- Session 元数据只放在 `sessionStorage`，用于离线刷新展示；HttpOnly Session Token 不暴露给 JavaScript。
- Service Worker 只提供静态壳。离线时节目详情使用本地占位，恢复网络后重新读取服务端数据。
- 真实 Provider、Nginx SSE、iOS Safari 安装态与系统 Share Sheet 仍属于后续目标环境验收，不能用本地 Fake Provider 或 Chromium 模拟替代。

## 本地验收证据

- 真实 Session 登录、刷新保活、注销和服务端撤销后统一回登录页通过。
- Import → Library → Note → Transcript → Search → AI → Export 主路径通过；禁用 LLM 时显示稳定错误且保留问题。
- 离线创建记录后刷新仍可见；恢复网络后数据库记录数与不同 `client_note_id` 数均为 1。
- Session 失效时 Outbox 保留，重新登录后自动重放且只落一条记录。
- 生产构建的 Service Worker 对 `/api` 使用导航回退拒绝规则，未配置 API 运行时缓存。
