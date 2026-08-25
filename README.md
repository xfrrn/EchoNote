# EchoNote MCP

EchoNote 是一个纯 MCP 听闻笔记服务：导入播客或音频、转写、记录笔记、搜索，并生成 AI 总结与对话。仓库不再包含网页、浏览器登录或 PWA；MCP 是唯一面向客户端的接口。

```text
MCP client ── STDIO / local Streamable HTTP ── apps/mcp
                                                   │
                                      loopback-only HTTP
                                                   │
                                     apps/server + PostgreSQL
```

## 目录

```text
apps/mcp/       MCP TypeScript 服务，提供 27 个工具
apps/server/    Go 内部 API、Worker、Migration 和 Job Queue
docs/           当前仍适用的算法说明
```

## 启动

要求 Node.js 22+、pnpm 10+、Go 1.25+、PostgreSQL（含 pgvector 与 `pg_trgm`）；转写还需要 FFmpeg/FFprobe。

1. 准备 PostgreSQL。没有现成实例时可使用仓库 Compose：

```powershell
docker compose up -d postgres
```

2. 设置内部服务配置。`ECHONOTE_OWNER_ID` 是稳定的数据分区 ID，更换后会看到另一份数据：

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable'
$env:ECHONOTE_OWNER_ID = '00000000-0000-4000-8000-000000000001'
```

3. 在一个终端启动内部 Go 服务：

```powershell
cd apps/server
go run ./cmd/api
```

内部 API 固定要求 `SERVER_HOST` 为回环 IP；默认监听 `127.0.0.1:8080`，没有登录、Cookie、Session 或开发环境免登录分支。development/test 会在 API 进程内同时启动 Worker 并自动应用 Migration。

4. 安装并构建 MCP：

```powershell
pnpm install
pnpm build
```

## 连接 Codex

在 `~/.codex/config.toml` 添加，并把路径替换为本仓库绝对路径：

```toml
[mcp_servers.echonote]
command = "node"
args = ["D:/codes/MyGithub/EchoNote/apps/mcp/dist/index.js"]
cwd = "D:/codes/MyGithub/EchoNote"
tool_timeout_sec = 180

[mcp_servers.echonote.env]
ECHONOTE_API_URL = "http://127.0.0.1:8080"
```

默认使用 STDIO。仅当客户端要求 Streamable HTTP 时运行 `pnpm mcp:http`，连接 `http://127.0.0.1:3001/mcp`；该端点只监听本机，不能直接公开到互联网。

可选的 ASR、对象存储、Embedding 和 LLM 配置见 [.env.example](.env.example)。

## 验证

```powershell
pnpm test
cd apps/server
go test ./...
go vet ./...
```
