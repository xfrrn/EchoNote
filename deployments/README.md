# EchoNote 原生生产部署

生产拓扑只使用 Nginx、systemd、独立 PostgreSQL 和云 Provider，不依赖 Docker。

- `nginx/`：同域 HTTPS、PWA 静态文件、API/SSE 代理、安全头、缓存和限速。
- `systemd/`：API、Worker、Migration、Maintenance 与显式启用的留存 Timer。
- `env/`：无真实密钥的环境模板；Production 与 Staging 必须分别填充。
- `postgres/`：管理员安装扩展，并配置 Runtime 与 Maintenance 最小授权。
- `monitoring/`：平台原生告警基线和只读健康/用量 SQL。
- `scripts/`：发布后 HTTPS/Session Smoke 与隔离恢复只读校验。
- `runbooks/`：部署、Migration、回滚、备份恢复、Provider Smoke、Release Gate 和日常运维步骤。

从 [deploy.md](runbooks/deploy.md) 开始。填充后的环境文件、证书、数据库 CA 和备份不得进入 Git。
