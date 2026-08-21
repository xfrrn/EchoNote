# EchoNote 生产就绪补全方案 v1.0

- 状态：待实施
- 基线：Phase 1–8，Git commit f981ddc
- 更新日期：2026-08-21

## 1. 结论

Phase 1–8 已完成后端业务能力，但当前仓库还不能直接作为生产产品交付。

阻塞生产发布的 P0 项只有四类：

1. Auth、Session 和请求级用户身份尚未实现。
2. Web 仍读取 Mock Data，没有接入真实 API。
3. 生产 HTTPS、进程托管、密钥和数据库权限尚未落地。
4. 真实云 Provider、监控、备份恢复和数据生命周期尚未完成验收。

必须完成本文 Phase 9–12，并通过最后的 Release Gate，才能将 EchoNote 标记为“生产可用”。

本文补充总体架构，不替代 Phase 1–8 实施记录，也不修改转录算法。

## 2. 当前基线

| 能力 | 当前状态 | 是否阻塞生产 |
| --- | --- | --- |
| API、Worker、PostgreSQL Job Queue | 已实现并通过真实 PostgreSQL 测试 | 否 |
| Migration、sqlc、OpenAPI | 已实现，生成物无漂移 | 否 |
| Import、Library、Notes、Transcription、Search、AI、Export | 后端已实现 | 否 |
| Provider 抽象 | Podcast、ASR、Embedding、LLM、Object Storage 已抽象 | 否 |
| 用户身份 | API 启动时注入固定 ECHONOTE_USER_ID | 是 |
| Web 数据源 | 业务页面仍依赖 shared/mock | 是 |
| 前后端契约 | packages/contracts 仍是占位 | 是 |
| 云端验收 | 只有本地 HTTP Contract Test 和 Fake Provider 测试 | 是 |
| 部署资产 | deployments 目录只有占位文件 | 是 |
| 监控和告警 | 有结构化日志与健康检查，没有上线告警闭环 | 是 |
| 备份恢复 | 没有可执行 Runbook 和恢复演练记录 | 是 |

## 3. 生产版范围

第一版生产环境采用以下边界：

- 单用户优先，但所有数据继续按 user_id 隔离。
- 用户由管理员创建，不开放公共注册。
- 用户名和密码登录，不引入邮件、短信或 OAuth 基础设施。
- PWA 与 API 同域部署。
- 保持 Go 模块化单体、API + Worker、PostgreSQL Job Queue。
- 使用独立 PostgreSQL 数据库、私有对象存储和现有云 Provider。
- 不引入 Redis、Kafka、微服务或 Kubernetes。
- 不实现播放器、播放历史、播放队列和音频流。

生产可用的最小定义：

1. 用户可以安全登录和退出。
2. 用户可从真实 Web 完成导入、记录、转录、搜索、AI 和导出。
3. API 或 Worker 重启后，数据和异步任务不会丢失。
4. 用户数据不会被另一用户读取。
5. 云端失败有稳定错误、可恢复路径和费用边界。
6. 数据库可以从备份恢复，并有真实演练记录。
7. 发生发布失败时，有不依赖生产 Down Migration 的回滚步骤。

目标拓扑：

    Browser / Installed PWA
              |
           HTTPS
              |
          Nginx
          /   \
     Static   /api
      Web      |
             API
              |
          PostgreSQL <---- Worker
                             |
                    Podcast / OSS / ASR
                    Embedding / LLM

## 4. Phase 9：Identity 与请求安全

### 4.1 目标

用真实 Session 用户替换固定 ECHONOTE_USER_ID，同时保留现有 Repository 的 user_id 查询边界。

第一版只支持管理员创建用户，不实现公开注册和邮件找回密码。

### 4.2 数据模型

新增 Migration，至少包含：

- users
  - id
  - username 和规范化后的唯一索引
  - password_hash
  - status
  - created_at、updated_at
- sessions
  - id
  - user_id
  - token_hash
  - expires_at
  - revoked_at
  - created_at、last_seen_at

密码使用专用密码哈希算法，参数在实现时通过目标服务器基准测试确定。Session Token 使用密码学安全随机数生成，数据库只保存 Token 的 SHA-256 摘要，不保存明文 Token。

Migration 必须保留现有业务表里的 user_id：

- 先为已有 user_id 回填不可登录的占位用户。
- 再添加业务表到 users 的外键。
- 管理命令显式认领并激活已有用户，不重写业务数据 ID。
- jobs.user_id 可以继续为空以容纳系统任务；业务 Job 必须有用户。

### 4.3 API

OpenAPI 新增：

- POST /api/v1/auth/login
- POST /api/v1/auth/logout
- GET /api/v1/me

除登录、健康检查外，所有业务 API 必须要求 Session。

登录成功设置 Cookie：

- Secure
- HttpOnly
- SameSite=Lax
- Path=/
- 不设置跨域 Domain
- 明确过期时间

所有修改数据的请求必须校验同源 Origin。登录错误不区分“用户不存在”和“密码错误”。登录入口在反向代理层做 IP 限速。

所有已认证 API 响应设置 Cache-Control: no-store，Service Worker 不得缓存用户数据或 Session 响应。

### 4.4 管理入口

增加最小管理员命令：

- 创建用户。
- 为占位用户设置用户名和密码。
- 重置密码并撤销该用户的全部 Session。

密码只能从交互式终端或标准输入读取，不能通过命令参数、环境变量或日志传递。

生产环境不允许把 ECHONOTE_USER_ID 当作请求身份。该变量只保留给 development 和 test。

### 4.5 预计修改

- Migration 和 sqlc Query
- internal/repository/auth
- HTTP Session Middleware 与 Auth Handler
- cmd/admin
- Config、OpenAPI、生成代码
- Auth 单元测试和 PostgreSQL HTTP 集成测试
- Phase 9 实施记录

不需要新增独立 Auth 服务、Redis Session 或 JWT。

### 4.6 验收

- 未登录访问任一业务 API 返回 401。
- 登录成功后 Cookie 属性正确，刷新页面仍保持登录。
- 退出、过期或撤销 Session 后立即返回 401。
- 非同源修改请求被拒绝。
- 两个测试用户不能互相读取 Episode、Note、Transcript、Search、AI 或 Export。
- 日志不包含密码、Session Token 或 Cookie。
- 既有 user_id 数据在 Migration 后仍可由对应用户访问。
- API 和 Worker 重启不影响有效 Session 或 Job。
- OpenAPI、Migration、sqlc、测试和文档同步更新。

### 4.7 回滚

Phase 9 Migration 必须保持加法兼容。应用回滚时保留 users 和 sessions 表，不把生产流量切回固定用户模式，也不运行生产 Down Migration。

## 5. Phase 10：Web 真实数据闭环

### 5.1 目标

用 OpenAPI 契约替换业务页面里的 Mock Data，使 PWA 可以完整使用后端。

Mock 只允许保留在 Design Playground 和测试中，不能进入生产业务路由。

### 5.2 API 契约

- packages/contracts 从 apps/server/openapi/openapi.yaml 生成 TypeScript 类型。
- 只保留一个生成命令，生成物提交 Git。
- CI 必须检查生成后工作树无差异。
- Web 使用同源 /api/v1，不维护第二份手写 DTO。
- 请求默认携带 Session Cookie，并统一解析稳定错误码。

### 5.3 页面接入顺序

按用户主路径接入：

1. 登录、退出和当前用户。
2. Library 列表、详情、删除和状态。
3. Apple Podcasts、RSS、Direct Audio 导入与状态轮询。
4. Capture 与 Notes。
5. Transcription 创建、进度 SSE、重试和取消。
6. Transcript 分页、Speaker 重命名和合并。
7. Keyword / Hybrid Search。
8. AI Artifact、Conversation、Citation 和 SSE。
9. Markdown、Clipboard 与 Web Share Export。

### 5.4 离线 Capture

离线能力先覆盖最重要的“新建记录”，不扩展到离线编辑和删除：

- 在发送前用 crypto.randomUUID 生成 client_note_id。
- 先写入浏览器本地 Outbox，再发起请求。
- 网络恢复后按创建顺序重放。
- 收到成功或幂等重放响应后才删除 Outbox 项。
- 4xx 业务错误停止自动重试并展示人工处理入口。
- 网络错误和 5xx 使用有上限的退避重试。
- 相同 client_note_id 永不用于另一条内容。

Outbox 使用浏览器原生 IndexedDB，不为了这一项引入新的同步基础设施。

### 5.5 PWA 与缓存

- Service Worker 只缓存版本化静态资源。
- index.html、Service Worker 文件使用可更新缓存策略。
- /api、SSE、登录和用户内容一律 Network Only。
- 登出时清除内存中的用户数据，但不静默删除尚未同步的 Outbox；用户必须能选择保留或清除。

### 5.6 验收

- 生产路由不再导入 shared/mock。
- TypeScript 构建和 OpenAPI 契约生成无漂移。
- 登录后能完成 Import → Library → Note → Transcription → Search → AI → Export。
- 浏览器离线时创建 Note，刷新页面后仍存在；恢复网络后只创建一条服务端记录。
- SSE 断线后可以恢复转录事件；AI 流失败会展示稳定错误而不是丢失已保存状态。
- Session 失效统一跳转登录，未同步 Outbox 不丢失。
- iOS PWA 和桌面浏览器至少各完成一次真实验收。

### 5.7 回滚

Web 静态资源必须可独立回滚到上一版本。后端 API 保持向后兼容至少一个 Web 版本；若契约不能兼容，必须先发布兼容后端，再发布 Web。

## 6. Phase 11：生产运行环境

### 6.1 目标

提供可重复部署、可重启、最小权限且同域 HTTPS 的运行环境。默认使用 Linux 原生进程和 Nginx，不要求 Docker。

### 6.2 部署资产

仓库需要补充：

- deployments/nginx：静态 Web、API 反向代理、SSE 和安全响应头。
- API 与 Worker 的进程托管配置。
- 环境变量模板，不包含真实密钥。
- deploy、rollback 和 migration Runbook。
- 发布后 Smoke Test 脚本。

API 与 Worker 使用独立的非 root 系统用户运行。可写目录只包含受控临时音频目录，其他应用文件只读。

### 6.3 HTTPS 与反向代理

- 只开放 443；80 仅重定向 HTTPS。
- PWA 和 /api 同域。
- TLS 证书自动续期并有到期告警。
- SSE 路由关闭代理缓冲和响应缓存，并设置足够的读取超时。
- 只对带内容哈希的静态资源设置长期缓存。
- 配置 CSP、HSTS、X-Content-Type-Options、Referrer-Policy 和 frame 限制。
- 登录和高成本写接口配置限速；健康检查不对公网暴露内部细节。

### 6.4 PostgreSQL

- 使用 EchoNote 独立数据库，禁止复用 autoup 或其他业务数据库。
- 生产应用不得使用 postgres 超级用户。
- Migration Role 可以变更 Schema；API / Worker Runtime Role 只能读写业务对象。
- 数据库连接强制 TLS 并校验证书。
- 目标实例预装与 PostgreSQL 版本匹配的 pg_trgm 和 pgvector。
- 连接池上限通过 pgx DSN 参数设置，并小于数据库连接预算。
- Migration 由独立发布步骤执行，不在 API 或 Worker 启动时自动执行。

### 6.5 密钥与 Provider

- Production、Staging 使用不同的数据库、Bucket、API Key 和账号权限。
- 密钥由部署系统或密钥管理服务注入，不进入 Git、镜像、日志和前端。
- OSS Bucket 必须私有，Key 只允许访问 EchoNote 前缀。
- ASR、Embedding、LLM Key 只授予所需模型与额度。
- Endpoint 固定为账号所属地域的 HTTPS 地址。
- 密钥轮换后不需要重新构建应用。

### 6.6 临时文件与对象生命周期

保持 Phase 5 语义：

- completed / canceled：删除源音频和预处理音频。
- completed：Chunk 在 72 小时后清理。
- failed：保留恢复材料，等待 Retry、Cancel 或 Episode 删除。
- raw ASR JSON：保留到 Episode 删除。

补充生产要求：

- Worker 启动时清理超出安全时限且不属于运行中任务的本地临时文件。
- 临时目录和对象存储使用量达到 80% 时告警。
- Episode 删除后的对象清理 Job 必须有失败告警和人工重试入口。

### 6.7 验收

- 一台全新主机可以只按 Runbook 完成部署。
- HTTPS、Cookie、安全响应头和同域路由检查通过。
- API、Worker 和主机重启后自动恢复。
- Worker 在任务执行中被终止，Lease 到期后任务可恢复且不重复持久化。
- SSE 经 Nginx 可持续工作并能断线恢复。
- 运行进程不能读取不属于自身的系统文件或使用数据库超级权限。
- 生产配置缺失时，发布前检查会失败，不允许带禁用的 ASR、Embedding 或 LLM 上线。
- 生产部署和验收不依赖 Docker。

### 6.8 回滚

发布前先备份数据库。失败时优先回滚 Web 和二进制，不执行 Down Migration。只有确认 Schema 与旧程序不兼容且恢复演练已通过时，才从发布前备份恢复到独立实例后切流。

## 7. Phase 12：云端、监控与灾备验收

### 7.1 真实 Provider 验收

Staging 使用目标云账号的小额度、独立 Key 完成：

- Apple Podcasts 单集解析。
- RSS 最新音频解析。
- Direct Audio 导入。
- Paraformer-v2 短音频转录。
- Fun-ASR 短音频转录。
- 包含两个 Speaker、超过一个 Core Window 的长音频验收。
- OSS 上传、Signed URL、ASR 拉取与生命周期清理。
- text-embedding-v4 生成和 Hybrid Search。
- Qwen Summary、Key Points、Speaker Views、Conversation、Citation 和 SSE。
- 四种 Export Mode。

长音频云端验收只在首次上线、转录算法变更或 Provider 大版本变化时执行，不作为每次发布的付费测试。

验收记录只保存请求 ID、模型、耗时、Token 或音频时长、结果状态和费用单位，不保存密钥或完整用户内容。

### 7.2 监控和告警

保持结构化日志，并补齐可聚合字段：

- request_id
- user_id
- episode_id
- job_id
- transcription_run_id
- provider
- operation
- duration_ms
- error_code
- input_tokens、output_tokens 或 audio_duration_ms

最低告警：

- API 5xx 比例持续异常。
- /readyz 连续失败。
- Worker 心跳消失。
- Job 排队时间超过对应类型阈值。
- failed Job 或 cleanup Job 出现。
- PostgreSQL 连接、磁盘或 CPU 超过 80%。
- Provider 401、429 或 5xx 持续出现。
- 当日 ASR、Embedding 或 LLM 用量超过预算。
- TLS 证书即将过期。
- 备份任务失败。

第一版可以使用部署平台的日志告警和 PostgreSQL 监控，不引入独立 Prometheus 集群。只有现有平台无法提供上述告警时，再增加指标基础设施。

### 7.3 备份和恢复

上线前确定并记录：

- 基线 RPO：24 小时。
- 基线 RTO：4 小时。
- PostgreSQL 每日备份保留 30 天。
- 支持时启用至少 7 天 PITR。
- OSS 对象版本或回收保护至少保留 7 天。
- 每季度恢复到隔离数据库并完成一次业务读取 Smoke Test。

恢复演练必须验证：

- Migration version 和 dirty 状态正确。
- Episode、Note、Transcript、Speaker、Search、AI Citation 关系完整。
- 未完成 Job 可以继续或明确取消。
- 恢复过程没有连接或修改 autoup 等其他数据库。

### 7.4 数据保留

第一版基线：

- 过期和撤销 Session：30 天后删除。
- succeeded / canceled Job 与 Event：30 天后删除。
- failed Job 与 Event：90 天后删除。
- 应用日志：30 天。
- Embedding：长期保存，可重建。
- Transcript、Note、AI Artifact：保留到 Episode 或用户删除。
- raw ASR JSON：保留到 Episode 删除。

清理任务必须按 user_id 或明确的系统范围执行，先提供 Dry Run 统计，再启用自动删除。

### 7.5 Release Gate

以下任何一项失败，都不能标记为生产可用：

功能：

- 真实 Web 完成登录和核心端到端流程。
- 三种导入来源可用。
- 两档 ASR、Speaker、Transcript Version 可用。
- Keyword / Hybrid Search 可用。
- AI 结果和 Citation 可核对。
- Export 可通过 Clipboard 或系统 Share Sheet 使用。

安全：

- 不存在固定生产用户身份。
- 所有业务 API 都需要 Session。
- 跨用户隔离、CSRF、SSRF 和登录限速测试通过。
- Cookie、TLS、CSP、私有 Bucket 和最小权限检查通过。
- Git、日志和前端产物中没有密钥。

数据：

- 全量 Migration 在隔离数据库从 0 升到最新版本。
- 生产升级只执行 Up Migration。
- 备份成功，恢复演练通过。
- Episode 删除和对象清理通过。

运行：

- API、Worker、PostgreSQL 和 Provider 都有告警。
- Worker 重启和 Job Lease 恢复通过。
- 发布、回滚、备份和 Provider Smoke Runbook 可由另一位工程师执行。

质量：

- go generate ./... 后工作树无差异。
- 使用独立测试数据库执行 go test ./...。
- go vet ./... 通过。
- pnpm build 通过。
- Web 浏览器端到端测试通过。
- Staging 真实 Provider Smoke Test 通过。

## 8. 环境隔离

| 环境 | 数据库 | Provider | 身份 | 用途 |
| --- | --- | --- | --- | --- |
| Development | 本地独立 echonote | Fake 或受控 Key | 可使用开发用户 | 日常开发 |
| Test | 独立测试库 | Fake Provider | 随机测试用户 | 自动化测试 |
| Staging | 独立云数据库和 Bucket | 限额真实 Key | 真实 Session | 发布前验收 |
| Production | 独立生产数据库和 Bucket | 生产 Key | 真实 Session | 用户流量 |

任何环境都不能使用 autoup 数据库。Staging 不复制未脱敏的生产用户内容。

## 9. 每个 Phase 的工作方式

Phase 9–12 继续遵循现有规则：

1. 先写“当前代码 → 目标 → 修改文件 → 风险”。
2. 只实现当前 Vertical Slice。
3. 同步 OpenAPI、Migration、sqlc、测试和文档。
4. 在独立数据库和目标运行环境完成验收。
5. 确认工作树只包含当前 Phase 变更。
6. 提交 Git 后才能进入下一 Phase。

建议提交边界：

- Phase 9：Identity 与请求安全。
- Phase 10：Web 真实数据闭环。
- Phase 11：生产运行环境。
- Phase 12：云端、监控与灾备验收。

## 10. P1：不阻塞首次生产发布

以下功能继续延期，直到有真实需求：

- 公共注册。
- 邮箱验证和邮件找回密码。
- Apple 登录或其他 OAuth。
- Settings 页面和多套用户偏好。
- 多设备 Session 管理 UI。
- 团队协作和管理员后台。
- 异步大文件 Export 和导出历史。
- 独立 Metrics 集群。
- 多区域、高可用和无停机数据库迁移。

## 11. 下一步

下一阶段应从 Phase 9 开始。Phase 9 验收并提交前，不开始 Web API 切换；否则前端会继续依赖固定用户身份，形成无法安全上线的中间状态。
