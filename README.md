# EchoNote MCP 转录服务

EchoNote 现在只有一个业务能力：接收播客页面、RSS、社交媒体帖子/列表页或直接音频 URL，异步转录，最终返回 Markdown。列表页会选择第一页中第一个含音频或视频的帖子。没有网页、笔记、搜索、AI 总结、对话或开发环境免登录入口。

```text
MCP 客户端 ── HTTPS + OAuth 2.1 ── apps/mcp :3001
                                      │ 内部密钥 + (issuer, subject)
                                      ▼
                              apps/server :8080（仅回环）
                                      │
                          PostgreSQL + Worker + ASR/OSS
```

MCP 暴露四个工具：

- `get_docs()`：返回完整使用指南。
- `transcribe_url(url)`：为播客页面、RSS、单条社交媒体帖子或直接音频 URL 创建异步任务并返回 `task_id`。
- `transcribe_profile_url(url)`：为社交媒体用户主页或列表页第一页中首个含音频或视频的帖子创建异步任务并返回 `task_id`。
- `get_transcription(task_id)`：查询进度；完成后直接返回 Markdown。

## 认证与多用户

EchoNote 是 OAuth 资源服务器，不保存密码。仓库提供自托管 Keycloak 配置，由 Keycloak 管理用户、登录、PKCE、令牌与密钥轮换；EchoNote 只验证 access token。默认关闭公开注册，由管理员创建用户，因此不是任何人都能使用。

- `iss`：与 `ECHONOTE_OAUTH_ISSUER` 完全一致。
- `sub`：稳定且不可变的用户 ID。
- `aud`：与 `ECHONOTE_OAUTH_AUDIENCE` 一致。
- `exp`：过期时间。
- `scope` 或 `scp`：包含 `echonote:transcribe`。

每个 `(iss, sub)` 首次调用时会自动创建独立用户；所有任务和转录结果按该用户隔离。`ECHONOTE_INTERNAL_TOKEN` 只用于 MCP 到回环 Go API 的服务间认证，不代表任何用户。

## 本地启动

要求 Docker、Node.js 22+、pnpm 10+、Go 1.25+、FFmpeg/FFprobe，以及阿里云 ASR 与 OSS 凭据。

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# 编辑 .env，填写 Keycloak 密码、ECHONOTE_INTERNAL_TOKEN、SnapAny、阿里云 ASR 与 OSS 凭据
docker compose up -d postgres keycloak
pnpm install
pnpm dev
```

`pnpm dev` 自动读取根目录 `.env`，同时启动 Go API、内嵌 Worker 和 MCP；按 `Ctrl+C` 会一起停止。PostgreSQL 与 Keycloak 仍由 Docker 管理。

打开 `http://127.0.0.1:8081/admin/`，使用 `KEYCLOAK_ADMIN_USERNAME` / `KEYCLOAK_ADMIN_PASSWORD` 登录，在 `echonote` realm 中创建测试用户并设置密码。随后配置并登录 Codex：

```powershell
codex mcp add echonote --url http://127.0.0.1:3001/mcp
codex mcp login echonote --scopes echonote:transcribe
```

登录命令会在浏览器打开本地 Keycloak。Realm 配置只在 Keycloak 数据库首次创建时导入；已有 realm 不会被启动命令覆盖。

## 远程部署

生产环境运行 Keycloak、`cmd/migrate`、`cmd/api`、`cmd/worker` 和 MCP。Keycloak 必须改用生产模式和独立 PostgreSQL，通过反向代理公开 `https://auth.example.com`；MCP 只公开 `https://mcp.example.com/mcp` 与 `/.well-known/oauth-protected-resource/mcp`。保持 `apps/server` 在 `127.0.0.1:8080`，不要公开数据库或内部 API。反向代理负责 TLS，并保留原始 `Host`。

```env
KEYCLOAK_PUBLIC_URL=https://auth.example.com
ECHONOTE_MCP_AUDIENCE=https://mcp.example.com/mcp
ECHONOTE_PUBLIC_URL=https://mcp.example.com/mcp
ECHONOTE_OAUTH_ISSUER=https://auth.example.com/realms/echonote
ECHONOTE_OAUTH_AUDIENCE=https://mcp.example.com/mcp
```

`ECHONOTE_MCP_AUDIENCE` 提供给 Keycloak 首次导入 realm；`ECHONOTE_OAUTH_AUDIENCE` 提供给 MCP 校验令牌。两者必须等于同一个公网 MCP URL。生产 Keycloak 使用同一个 `ops/keycloak/echonote-realm.json` 首次导入，但启动命令应使用 `start --features=cimd`，不能使用 Compose 中仅供本地测试的 `start-dev`。上线前替换管理员和数据库密码，并在 Keycloak 中配置 SMTP、密码找回与需要的 MFA 策略。

```powershell
$env:APP_ENV = 'production'
cd apps/server
go run ./cmd/migrate up
go run ./cmd/api       # 单独终端
go run ./cmd/worker    # 单独终端

cd ../..
pnpm mcp               # 单独终端
```

若 MCP 与反向代理不在同一网络命名空间，可设置 `ECHONOTE_MCP_HOST=0.0.0.0`，但仍只允许代理访问该端口。

Codex 连接配置：

```toml
[mcp_servers.echonote]
url = "https://mcp.example.com/mcp"
```

完整变量见 [.env.example](.env.example)。Migration 10 会永久删除旧笔记、搜索、AI 和播客容器数据，升级已有数据库前先备份。

## 验证

```powershell
pnpm test
cd apps/server
go test ./...
go vet ./...
```
