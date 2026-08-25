# EchoNote 内部转录服务

该 Go 模块只提供 MCP 所需的私有转录 API、任务队列和 Worker。API 必须监听回环地址，并要求 `ECHONOTE_INTERNAL_TOKEN` 以及 MCP 注入的已验证 OAuth 身份。

## 进程

```powershell
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/worker
```

development/test 下 `cmd/api` 会自动迁移并内嵌 Worker；staging/production 必须单独运行三个进程。必填配置见仓库根目录 [.env.example](../../.env.example)。

内部 HTTP 契约只有：

- `POST /api/v1/transcriptions`
- `GET /api/v1/transcriptions/{taskId}`
- `GET /healthz`
- `GET /readyz`

不要将 `SERVER_HOST` 改为公网地址。公网 OAuth 校验位于 `apps/mcp`，Go API 的内部密钥用于阻止其他本机进程伪造身份。

## 用户运维

OAuth 用户首次访问会自动创建。停用用户可把 `users.status` 设为 `disabled`。如需把旧数据绑定到 OAuth 身份：

```powershell
go run ./cmd/admin bind-identity <user-id> <issuer> <subject>
```

Migration 10 是不可逆的数据裁剪；升级已有数据库前先备份。

## 验证

```powershell
go generate ./...
go test ./...
go vet ./...
```
