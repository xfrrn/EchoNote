# EchoNote — 高保真 PWA Demo

EchoNote 是一个“智能听闻笔记本”的第一版前端 Demo，用于验证视觉设计、信息架构、移动端交互与 iPhone PWA 体验。

本仓库不包含后端、数据库、登录、真实 ASR 或真实 AI。所有数据均为本地 Mock。

## 启动与构建

```bash
pnpm install
pnpm dev
```

生产构建：

```bash
pnpm build
pnpm preview
```

## PWA 部署说明

1. 执行 `pnpm build`，将 `dist/` 部署到任意 HTTPS 静态托管。
2. 必须部署在域名根路径 `/`。如果放在子路径，需要同步修改 `vite.config.ts` 中的 `base`、manifest 图标路径与 `start_url`。
3. 静态服务器建议配置 SPA fallback，将未知路径回退到 `index.html`。Service Worker 安装后，Workbox 的 `navigateFallback` 也会处理离线/刷新导航。
4. iPhone 使用 Safari 打开 HTTPS 地址 → 分享 → 添加到主屏幕。
5. 从主屏幕打开后，应用以 `standalone` 模式运行，并启用 `viewport-fit=cover` 与 `safe-area-inset-*` 适配。

### 深色启动画面

- `manifest.webmanifest` 为 Light Mode，`manifest-dark.webmanifest` 为 Dark Mode。
- `index.html` 的内联脚本会在首帧前根据用户主题选择 manifest、`theme-color` 与状态栏样式。
- 已提供 11 组常见 iPhone 尺寸的 `apple-touch-startup-image`，Light / Dark 各一套，覆盖从 iPhone SE 到 Pro Max 的常用机型。
- 启动图与 manifest 会进入 Service Worker 预缓存。

## 目录结构

```text
src/
├── app/
│   ├── router/          # React Router 路由
│   ├── layout/          # AppShell 与 Bottom Navigation
│   └── providers/       # QueryClientProvider、主题同步
├── features/
│   ├── library/         # /library
│   ├── episode/         # /episode/:id 笔记 / Transcript / AI
│   ├── capture/         # /capture 快速记录
│   ├── search/          # /search 全局搜索
│   ├── mine/            # /mine 测试模式开关
│   └── design/          # /dev/design Design Playground
├── shared/
│   ├── components/      # 跨 Feature 基础组件
│   ├── hooks/           # useResolvedTheme
│   ├── types/           # 领域类型
│   ├── mock/            # 中文 Mock 数据与选择器
│   └── store/           # Zustand：测试模式、Capture 草稿
└── styles/
    ├── tokens.css       # Light / Dark Semantic Tokens
    └── globals.css      # Tailwind、Glass、Safe Area、Highlight
```

## Design Tokens

核心 Semantic Token（组件只引用变量，不直接写 hex）：

```text
--bg-primary / --bg-surface / --bg-secondary
--text-primary / --text-secondary / --text-tertiary
--separator / --separator-strong
--accent / --accent-active / --accent-soft / --on-accent
--danger / --success
--glass-bg / --glass-blur / --glass-saturate
--overlay / --shadow-control / --shadow-sheet
--radius-sm(8) / --radius-md(12) / --radius-lg(16) / --radius-xl(20)
```

Light 背景为 `#f7f7f8`，Dark 背景为 `#0a0a0a`。Dark 模式通过 `html.dark` 切换 Semantic Token，并同步 `theme-color` 与 `color-scheme`。

字体使用系统字体栈；页面左右间距 16px；Spacing 全部落在 4px grid；Transcript 正文 17px / 1.65。

## 测试模式

入口：底部导航 → 我的。

可切换：

- Light / Dark / System
- 短标题 / 超长标题
- 少量 / 大量 Transcript
- 1 / 2 / 4 位 Speaker
- 无 / 少量 / 大量笔记
- 转录成功 / 转录中 / 失败

Design Playground 位于 `/dev/design`，也可从「我的」进入。

## 本轮有意的小调整

1. **Capture 页隐藏 Bottom Navigation**：记录是瞬时、专注的交互。进入记录页后保留底部导航会分散注意力，也更容易在键盘弹起时误触，因此记录页采用全屏专注模式。
2. **Speaker 测试增加 2 位选项**：产品默认场景是主持人与嘉宾两人对话；规范要求测试 1 / 4 位，但缺省需要落在 2 位，所以测试开关提供了 1 / 2 / 4。
