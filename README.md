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
go run ./cmd/worker
```

API 和 Worker 启动时会自动检查并补齐数据库 Schema，不需要单独执行 Migration 命令；首次初始化使用的数据库账号需要有目标 Schema 的建表权限。优先使用已有 PostgreSQL；仓库根目录的 Compose 仅作为没有本地数据库时的可选方案。详细启动、Worker、代码生成和测试说明见 [`apps/server/README.md`](apps/server/README.md)。
