# EchoNote

EchoNote 是一个“智能听闻笔记本”：导入播客，自动转写，沉淀笔记，并提供 AI 总结与对话。

本仓库为 **Monorepo**，目前包含已可运行的 Web 前端（高保真 PWA Demo）与规划中的 Go 后端骨架。

## 仓库结构

```text
echonote/
├── apps/
│   ├── web/            # Web 前端（Vite + React + TS + Tailwind，PWA）
│   └── server/         # Go 后端（骨架：cmd/api、cmd/worker、internal/...）
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

## 后端（规划中）

`apps/server` 为 Go 服务骨架，目录已按 DDD 风格划分（`cmd` / `internal/{domain,service,provider,repository,database,worker}` / `migrations`），业务代码尚未实现。见 [`apps/server/README.md`](apps/server/README.md)。
