# EchoNote

EchoNote 是一个“智能听闻笔记本”：导入播客，自动转写，沉淀笔记，并提供 AI 总结与对话。

本仓库为 **Monorepo**，目前包含已可运行的 Web 前端（高保真 PWA Demo）与完成 Phase 9 Identity 的 Go 后端。

## 仓库结构

```text
echonote/
├── apps/
│   ├── web/            # Web 前端（Vite + React + TS + Tailwind，PWA）
│   └── server/         # Go 后端（API、Worker、Migration、Job Queue）
├── packages/
│   └── contracts/      # 前后端共享的 API 契约与类型（占位）
├── docs/
│   ├── product/  design/  architecture/
├── deployments/
│   ├── docker/  nginx/
├── scripts/
├── .env.example
└── pnpm-workspace.yaml
```

## 快速开始（前端）

前端在 `apps/web`，使用 pnpm workspace 管理。仓库根目录可直接执行：

```bash
pnpm install
pnpm dev        # 启动 apps/web 开发服务器
pnpm build      # 生产构建（tsc -b && vite build）
pnpm preview    # 预览构建产物
```

> 详见 [`apps/web/README.md`](apps/web/README.md)（PWA 部署、Design Tokens、测试模式等）。

## 后端（Phase 9）

`apps/server` 已完成基础设施、业务垂直切片，以及用户名密码登录、数据库 Session、CSRF Origin 校验和管理员用户命令。Web Demo 暂未从 Mock Data 切换到后端；生产发布前还需完成真实 Web 接入、部署安全和灾备验收，详见 [生产就绪补全方案](docs/architecture/production-readiness.md)。

```bash
cd apps/server
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable'
go run ./cmd/api
```

开发环境的 API 会在同一进程启动 Worker，并自动检查和补齐数据库 Schema，因此日常只需要前端与后端两个终端。首次初始化使用的数据库账号需要有目标 Schema 的建表权限。优先使用已有 PostgreSQL；仓库根目录的 Compose 仅作为没有本地数据库时的可选方案。详细启动、Worker、代码生成和测试说明见 [`apps/server/README.md`](apps/server/README.md)。

## MCP 服务

`apps/mcp` 使用官方 MCP TypeScript SDK，把现有 Go API 暴露为 27 个带输入校验和安全标注的工具；不包含网页 UI。先启动后端并构建 MCP：

```bash
pnpm --filter @echonote/mcp build
```

本机 Codex / ChatGPT 桌面端推荐 STDIO，让客户端自动启动进程。在 `~/.codex/config.toml` 添加（把路径替换为本仓库绝对路径）：

```toml
[mcp_servers.echonote]
command = "node"
args = ["D:/codes/MyGithub/EchoNote/apps/mcp/dist/index.js"]
cwd = "D:/codes/MyGithub/EchoNote"
tool_timeout_sec = 180

[mcp_servers.echonote.env]
ECHONOTE_API_URL = "http://127.0.0.1:8080"
```

开发环境配置 `ECHONOTE_USER_ID` 时无需 MCP 登录信息。生产后端还需在主机环境设置 `ECHONOTE_API_ORIGIN`、`ECHONOTE_USERNAME` 和 `ECHONOTE_PASSWORD`，并在服务表加入 `env_vars = ["ECHONOTE_API_ORIGIN", "ECHONOTE_USERNAME", "ECHONOTE_PASSWORD"]` 转发，避免把密码写入配置文件。若客户端只支持 Streamable HTTP，运行 `pnpm mcp:http`，连接 `http://127.0.0.1:3001/mcp`。该 HTTP 端点只监听本机；远程部署需要 OAuth 2.1，不能直接公开。
