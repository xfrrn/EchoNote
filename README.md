# EchoNote

EchoNote 是一个“智能听闻笔记本”：导入播客，自动转写，沉淀笔记，并提供 AI 总结与对话。

本仓库为 **Monorepo**，目前包含已可运行的 Web 前端（高保真 PWA Demo）与完成 Phase 4 Notes 的 Go 后端。

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

## 后端（Phase 4）

`apps/server` 已完成基础设施、Apple Podcasts/RSS/直接音频导入、Episode Library，以及 Capture、Notes、离线重试幂等与软删除。Transcription 等业务仍按后续 Vertical Slice 实现。

```bash
cd apps/server
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable'
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/worker
```

优先使用已有 PostgreSQL；仓库根目录的 Compose 仅作为没有本地数据库时的可选方案。详细启动、Worker、代码生成和测试说明见 [`apps/server/README.md`](apps/server/README.md)。
