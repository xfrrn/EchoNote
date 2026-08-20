# 后端服务（Go）

EchoNote 的后端：播客源接入、转写（ASR）、搜索、AI 对话与总结。

> 当前为**占位骨架**。目录结构已按架构设计划好，业务代码尚未实现。

## 目录约定

```text
cmd/
  api/            # HTTP API 服务入口（main）
  worker/         # 异步任务进程入口（转写 / AI 等）
internal/
  config/         # 配置加载
  http/           # HTTP 路由、handler、中间件
  domain/         # 领域模型与接口（不依赖外部实现）
    podcast/ episode/ transcript/ note/ search/ conversation/
  service/        # 应用服务（用例编排）
    podcast/ transcription/ search/ ai/
  provider/       # 外部能力适配（实现 domain 定义的接口）
    podcast/      #   apple / xiaoyuzhou / rss
    asr/ llm/ embedding/ storage/
  repository/     # 数据访问（持久化）
  database/       # 连接、迁移执行
  worker/         # 异步任务消费者
migrations/       # SQL 迁移文件
```

- 依赖方向：`cmd → internal/{http,service} → domain`，`provider`/`repository` 在边界实现 `domain` 的接口（依赖反转）。
- `internal/` 下的包对外不可导入，保证边界清晰。

落地时在此初始化模块：

```bash
go mod init github.com/Actify/echonote/apps/server
```
