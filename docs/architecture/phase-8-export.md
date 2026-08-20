# Phase 8：Export 实施记录

更新时间：2026-08-21

## 范围

本阶段按垂直切片完成：

- Notes-only Export
- AI Organized Note Export
- Selected Transcript Export
- Full Transcript Export
- Apple Notes 友好的纯文本 Share Content
- Markdown 内容与安全建议文件名
- 一致性快照、来源校验和同步大小边界

没有实现后端直写 Apple Notes、导出历史、导出文件对象存储、异步 Export Job、Settings 或前端 API 切换。Web Demo 已有的本地 Mock Composer 暂时保留，后续接入时只需用本阶段 API 返回的 `text` 调用 Clipboard / Web Share。

## 实现前分析

现有前端 `ShareSheet` 已明确正确职责：

```text
后端生成内容
→ PWA 调用系统 Share Sheet
→ 用户选择 Apple 备忘录
```

后端已有 Episode、Source、Notes、active Transcript 与 ready AI Artifact，导出不需要新的业务真相。最小路径因此是同步读取一致快照并格式化：

```text
POST Export
→ PostgreSQL read-only repeatable-read snapshot
→ 校验 ready Artifact / active Transcript / selected Segment
→ 生成 Apple Notes 纯文本
→ 生成 Markdown
→ 返回 JSON，不持久化
```

没有增加 Apple Notes Provider。Apple Notes 没有由服务端替用户无交互写入个人备忘录的项目能力，把 Share Sheet 包装成 Provider 会混淆职责。

## 为什么没有 Migration v7

整体设计明确第一版 Export 可以无状态。本阶段没有新增 Schema，数据库仍停留在 Migration v6；`sqlc` 只增加只读 Export Query。原因是：

- 相同输入可以廉价、确定性地重新排版。
- 当前没有审计、下载历史或异步大文件需求。
- 保存 `text` / `markdown` 会复制 Notes、Transcript 与 Artifact，带来额外失效和隐私清理路径。
- 对象存储只应在真实需要异步大文件下载时引入。

达到“超过同步上限仍必须导出”或“用户需要历史下载链接”的产品条件后，再新增 Export Job、记录表和临时对象生命周期；本阶段不为假设需求预建。

## API

OpenAPI 0.8.0 新增：

```text
POST /api/v1/episodes/{episode_id}/exports
```

响应始终同时返回：

```json
{
  "title": "节目｜单集标题",
  "text": "适合复制或传给 iOS Share Sheet 的纯文本",
  "markdown": "# 可保存为 .md 的内容",
  "suggested_filename": "节目｜单集标题.md"
}
```

后端不接受目标 App，也不声称已经写入 Apple Notes。

## 四种 Mode

### `notes_only`

只导出当前用户未软删除的 Notes。没有 Notes 时返回 `409 EXPORT_CONTENT_EMPTY`。

### `organized_note`

可选择：

```text
include_user_notes
include_summary
include_key_points
include_worth_reviewing
include_transcript
```

前四项省略时默认 `true`，`include_transcript` 默认 `false`，与当前前端 Share Sheet 一致。若请求任一 AI Section，只读取当前 `ready` Artifact；只有 stale / failed / queued 结果时返回 `409 AI_ARTIFACT_NOT_READY`，不会自动触发付费 LLM。

`include_transcript=true` 时，有 `transcript_segment_ids` 就按选中 Segment 导出；没有 ID 时只取 active Transcript 前四段，保持当前前端“Transcript 节选”语义。

### `selected_transcript`

要求 1–200 个唯一 `transcript_segment_ids`。所有 ID 必须属于该用户、该 Episode 的 active Transcript；任一缺失或来自旧 Version / 其他 Episode 时，整体返回 `400 TRANSCRIPT_SEGMENTS_NOT_FOUND`，不会泄漏 ID 是否存在。

响应按 Transcript 原始 `sequence` 排列，不按请求 ID 顺序排列。

### `full_transcript`

导出 active Transcript 全部 Segment。没有 active Transcript 时返回 `409 TRANSCRIPT_NOT_READY`。

## 内容格式

纯文本使用 Apple Notes 中易读的轻量结构：

```text
节目｜单集标题
时长 · 发布日期
原始链接

【一句话总结】
……

【核心观点】
1. ……

【Transcript 节选】
Speaker 00:14:44：……

—— 用 EchoNote 整理
```

Markdown 使用 Heading、列表和 Blockquote。Episode、AI、Note 与 Transcript 文本先规范控制字符和换行；Markdown 特殊字符会转义，Transcript 放在 Quote 中，避免播客文本改变文档结构。时间由 `start_ms` 确定性格式化为 `HH:MM:SS`，不生成播放器或音频链接。

建议文件名保留 Unicode，替换 Windows / Unix 禁止字符，处理设备保留名，去除尾随点和空格，并把主体限制为 100 个 Unicode 字符。

## 一致性与边界

Repository 使用 PostgreSQL `REPEATABLE READ READ ONLY` Transaction，一次 Export 中的 Episode、Notes、Artifact 与 Transcript 来自同一快照。这样 Note 修改与 Artifact stale 的原子事务不会出现“新 Note + 旧 ready Artifact”的混合导出。

完整 Transcript 在加载正文前先由 SQL 统计选中 Segment 数量与 UTF-8 Bytes：

- 原文超过 4 MiB 时提前返回 `413 EXPORT_TOO_LARGE`。
- 格式化后的任一 `text` / `markdown` 超过 4 MiB 时同样返回 413。
- 不静默截断 Full Transcript。

Response 带 `Cache-Control: no-store`。所有 Episode、Artifact、Transcript 和 Segment Query 都包含用户所有权边界。

## 测试

自动验收覆盖：

- organized note 同时生成五类可选 Section。
- Apple Notes 文本、Markdown 转义、时间格式和文件名清洗。
- 4 MiB 同步边界。
- ready Artifact 成功导出；stale Artifact 返回 409。
- selected Segment 越权 / 不属于 active Transcript 时原子拒绝。
- Full Transcript、Notes-only 与空 Section 校验。
- 跨用户 Episode 返回 404。
- Phase 1–7 全量 PostgreSQL 回归、`go vet` 与 Web 构建。

本地测试继续使用独立 `echonote` 数据库，没有访问 `autoup`、没有启动 Docker，也没有调用任何外部 Provider。
