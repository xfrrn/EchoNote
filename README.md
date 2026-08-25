# EchoNote MCP 转录服务

EchoNote 现在只有一个业务能力：接收播客页面、RSS 或直接音频 URL，异步转录，最终返回 Markdown。没有网页、笔记、搜索、AI 总结、对话或开发环境免登录入口。

```text
MCP 客户端 ── HTTPS + OAuth 2.1 ── apps/mcp :3001
                                      │ 内部密钥 + (issuer, subject)
                                      ▼
                              apps/server :8080（仅回环）
                                      │
                          PostgreSQL + Worker + ASR/OSS
```

MCP 只暴露两个工具：

- `transcribe_url(url)`：创建异步任务并返回 `task_id`。
- `get_transcription(task_id)`：查询进度；完成后直接返回 Markdown。

## 认证与多用户

EchoNote 是 OAuth 资源服务器，不自行保存密码或实现登录页面。你需要一个外部 OAuth 2.1/OIDC 服务（如自建 Keycloak 或托管身份平台），它必须提供 HTTPS discovery、JWKS、Authorization Code + PKCE S256，并签发包含以下内容的 JWT access token：

- `iss`：与 `ECHONOTE_OAUTH_ISSUER` 完全一致。
- `sub`：稳定且不可变的用户 ID。
- `aud`：与 `ECHONOTE_OAUTH_AUDIENCE` 一致。
- `exp`：过期时间。
- `scope` 或 `scp`：包含 `echonote:transcribe`。

每个 `(iss, sub)` 首次调用时会自动创建独立用户；所有任务和转录结果按该用户隔离。`ECHONOTE_INTERNAL_TOKEN` 只用于 MCP 到回环 Go API 的服务间认证，不代表任何用户。

## 本地启动

要求 Node.js 22+、pnpm 10+、Go 1.25+、PostgreSQL 17、FFmpeg/FFprobe，以及阿里云 ASR 与 OSS 凭据。

```powershell
docker compose up -d postgres
pnpm install

$env:DATABASE_URL = 'postgres://postgres:postgres@127.0.0.1:5432/echonote?sslmode=disable'
$env:ECHONOTE_INTERNAL_TOKEN = '<至少 32 字符的随机密钥>'
$env:ASR_PROVIDER = 'aliyun'
$env:ASR_API_KEY = '<key>'
$env:STORAGE_PROVIDER = 'aliyun_oss'
$env:STORAGE_REGION = 'cn-beijing'
$env:STORAGE_BUCKET = '<bucket>'
$env:STORAGE_ACCESS_KEY = '<access-key>'
$env:STORAGE_SECRET_KEY = '<secret-key>'
cd apps/server
go run ./cmd/api
```

development/test 模式下 API 会自动迁移数据库并在同一进程启动 Worker。另开终端，注入相同的内部密钥和 OAuth 配置后启动 MCP：

```powershell
$env:ECHONOTE_INTERNAL_TOKEN = '<与 API 相同的随机密钥>'
$env:ECHONOTE_API_URL = 'http://127.0.0.1:8080'
$env:ECHONOTE_PUBLIC_URL = 'https://mcp.example.com/mcp'
$env:ECHONOTE_OAUTH_ISSUER = 'https://login.example.com/'
$env:ECHONOTE_OAUTH_AUDIENCE = 'https://mcp.example.com/mcp'
pnpm mcp
```

## 远程部署

生产环境运行 `cmd/migrate`、`cmd/api`、`cmd/worker` 和 MCP 四个进程。只通过反向代理公开 `https://mcp.example.com/mcp` 与 `/.well-known/oauth-protected-resource/mcp`；保持 `apps/server` 在 `127.0.0.1:8080`，不要公开数据库或内部 API。反向代理负责 TLS，并保留原始 `Host`。

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
