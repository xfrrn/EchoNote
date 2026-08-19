# EchoNote 前端 UI / UX 设计规范 v0.1

## 1. 产品视觉定位

关键词：

**安静 / 克制 / 清晰 / 轻盈 / 内容优先 / 原生感**

不要做成传统 SaaS：

- 不使用大面积渐变
- 不使用大量彩色卡片
- 不堆阴影
- 不做复杂 Dashboard
- 不使用粗重边框
- 不到处放 Badge
- 不把 AI 做成满屏紫色渐变

整体视觉更接近：

**Apple Notes × Apple Podcasts × Safari Reading Mode**

但信息组织能力更强。

---

# 2. 核心设计原则

### Content First

界面本身退后，让这些东西成为视觉主体：

**播客标题、你的记录、Transcript、AI 结果。**

用户进入 Episode 页面后，第一眼应该看到内容，而不是工具栏。

### Progressive Disclosure

默认只展示最重要的信息。

例如 Episode 首页：

**节目标题 → 我的记录 → AI 摘要**

全文 Transcript、Speaker、元数据等通过 Tab 或二级操作查看。

### Quiet UI

大量使用留白。

一个页面最多存在一个明显的 Primary Action。

例如：

**＋ 导入**

或者：

**记录想法**

不能同时出现五六个蓝色按钮。

---

# 3. 色彩系统

## Light Mode

| Token | 色值 | 用途 |
|---|---|---|
| `--bg-primary` | `#F7F7F8` | 页面背景 |
| `--bg-surface` | `#FFFFFF` | 内容 Surface |
| `--bg-secondary` | `#F1F1F3` | 次级区域 |
| `--text-primary` | `#1D1D1F` | 主要文字 |
| `--text-secondary` | `#6E6E73` | 次级信息 |
| `--text-tertiary` | `#98989D` | 时间、辅助信息 |
| `--separator` | `rgba(60,60,67,.12)` | 分隔线 |
| `--accent` | `#0A78F0` | 主交互色 |
| `--danger` | `#FF453A` | 删除/错误 |
| `--success` | `#30A46C` | 完成状态 |

整个产品原则：

**90% 中性色 + 10% 强调色。**

蓝色只用于：

链接、选中状态、Primary Action、可点击重点。

---

# 4. 字体规范

网页不要自行打包 SF Pro 字体。

统一：

```css
font-family:
  system-ui,
  -apple-system,
  BlinkMacSystemFont,
  "Segoe UI",
  sans-serif;
```

在 iPhone Safari/PWA 上自然使用系统字体，从而获得接近原生 iOS 的排版。Apple 当前 iOS/iPadOS 系统字体为 SF Pro，并建议利用系统字体形成一致的视觉体验。

字号体系：

| 类型 | Size | Weight | 用途 |
|---|---:|---:|---|
| Large Title | 32px | 700 | 页面主标题 |
| Title 1 | 26px | 700 | Episode 标题 |
| Title 2 | 21px | 600 | Section |
| Headline | 17px | 600 | 卡片标题 |
| Body | 17px | 400 | Transcript / 正文 |
| Callout | 16px | 400 | AI 内容 |
| Subheadline | 15px | 400 | 辅助信息 |
| Caption | 13px | 400 | 时间、状态 |

Transcript：

```css
font-size: 17px;
line-height: 1.65;
```

阅读体验优先，不追求信息塞满屏幕。

---

# 5. 间距系统

统一采用 **4px Grid**。

```text
4
8
12
16
20
24
32
40
48
```

主要规则：

页面左右：

**16px**

大型区域之间：

**32px**

标题 → 内容：

**12px**

列表 Item：

**12–16px**

正文段落：

**12px**

不要随意出现：

`13px / 17px / 27px`

这种随机 spacing。

---

# 6. 圆角系统

统一只允许：

```text
8px   小型控件
12px  Input / Button
16px  Card
20px  Sheet / Modal
24px  大型浮层
999px Pill
```

默认 Card：

```css
border-radius: 16px;
```

避免所有东西都做成巨大圆角。

---

# 7. 阴影

Apple 风格的关键不是“大阴影”，而是**层级非常轻**。

普通 Card：

**无 Shadow。**

主要依靠：

```text
Background
+
Surface
+
Separator
```

浮层才使用：

```css
box-shadow:
  0 8px 30px rgba(0,0,0,.08);
```

---

# 8. Glass / Blur

只允许出现在：

**Bottom Navigation、顶部 Floating Toolbar、Modal / Sheet。**

例如：

```css
background: rgba(250,250,250,.78);

backdrop-filter:
  blur(24px)
  saturate(180%);
```

内容卡片禁止使用 Glass。

原则：

> Glass 是 Control Layer，而不是 Content Layer。

---

# 9. Button

## Primary

高度：

**48px**

样式：

```text
蓝色背景
白色文字
12px Radius
```

例如：

**开始转录**

## Secondary

```text
浅灰背景
深色文字
```

例如：

**重新识别**

## Plain

无背景：

**分享 / 编辑 / 更多**

所有核心点击区域至少保持约 **44×44pt** 的可触达面积，这与 Apple 当前按钮可访问性建议一致。

---

# 10. Card

不要传统 SaaS 风格：

```text
┌─────────────────────┐
│ 🎙 Podcast          │
│                     │
│ Title               │
│ 一堆标签             │
│                     │
│ [详情] [编辑] [AI]   │
└─────────────────────┘
```

改成 Content Card：

```text
硅谷101

E248 一个“催发货”AI要跑通260步

今天
3 条记录 · 已转录

                              ›
```

基本没有装饰。

**内容本身就是 UI。**

---

# 11. Bottom Navigation

移动端主导航：

```text
节目        记录        搜索        我的
```

推荐对应概念图标：

```text
Library
Compose
Search
Profile
```

高度约：

**80px + safe-area**

采用轻微 Blur。

当前选中项：

```text
Icon：Accent
文字：Accent / Semibold
```

其他：

```text
#8E8E93
```

不做传统 Web 的巨大蓝色 Footer。

---

# 12. 首页 Library

顶部：

```text
19:41

EchoNote                       ＋
```

下面可以有：

```text
最近

硅谷101
E248 一个“催发货”AI要跑通260步
3 条记录 · 已转录
────────────────────────────

原点 The Origin
23岁在硅谷融资千万美金
1 条记录 · 转录中
────────────────────────────
```

注意：

**首页优先用 List，而不是 Card Grid。**

因为这是内容工具，不是电商应用。

---

# 13. Episode 页面

这是整个产品视觉核心。

结构：

```text
‹ 节目

硅谷101

E248 一个“催发货”AI要跑通260步，
和阿里瓴羊朋新宇聊聊中国式FDE

2026年8月19日 · 64分钟

已转录
```

然后：

```text
笔记      Transcript      AI
```

使用轻量 Segment Control。

---

## 我的笔记

不是一个个厚重 Card。

使用 Timeline：

```text
19:32

这里 FDE 的定义和我之前理解的不一样


19:45

260个步骤这个案例值得记一下


20:03

可以问一下 AI：中国式 FDE 和美国有什么区别？
```

记录之间使用大量留白。

---

# 14. Transcript

重点是阅读体验。

形式：

```text
朋新宇
00:32:18

其实我们当时遇到的问题是，
企业的软件系统已经非常复杂……



主持人
00:32:46

所以这里其实不是传统意义上的
SaaS 对吧？
```

Speaker：

**15px / Semibold**

时间：

**13px / Tertiary**

正文：

**17px / 1.65 line-height**

不要气泡。

不要：

```text
蓝色框 = Speaker A
灰色框 = Speaker B
```

Transcript 应该像一篇采访稿。

---

# 15. AI 页面

AI 不能设计成 ChatGPT 克隆。

默认首先显示：

```text
AI 整理

一句话总结

核心观点

人物观点

值得回顾

我的笔记和节目内容有什么联系
```

下面才出现：

```text
问这期节目…
```

用户主动进入问答后才变 Chat UI。

---

# 16. 极速记录 Capture

这是最需要“原生感”的页面。

打开：

```text
取消                  完成


硅谷101
E248 · 正在记录


刚才关于 FDE 的定义
非常值得重新研究一下


+
```

没有复杂 Toolbar。

没有：

```text
标题
分类
标签
文件夹
时间戳
优先级
```

核心指标：

> 从打开页面到开始输入，不超过一次点击。

---

# 17. 搜索

顶部使用大型 Search Field：

```text
⌕  搜索播客、内容和笔记
```

结果按照内容分类：

```text
我的笔记

硅谷101
这里 FDE 的定义……
────────────────────────────


Transcript

硅谷101 · 32:18
FDE需要真正进入企业……
────────────────────────────
```

搜索词在结果中轻量 Highlight。

不要黄色马克笔式高亮。

---

# 18. Motion

动画统一：

```text
150ms  快速反馈
220ms  普通过渡
300ms  Sheet / Page
```

Easing：

```css
cubic-bezier(.2,.8,.2,1)
```

原则：

**动画体现物理关系，不体现“炫技”。**

例如：

点击 Episode：

```text
List
  ↓
内容自然展开
  ↓
Episode
```

而不是旋转、弹跳、缩放特效。

---

# 19. Dark Mode

从第一版就支持。

不是简单：

```text
白 → 黑
```

而是：

```text
Background    #000000 / #0A0A0A
Surface       #1C1C1E
Secondary     #2C2C2E

Text Primary  #F5F5F7
Secondary     #A1A1A6
Separator     rgba(84,84,88,.45)
```

颜色全部使用 Semantic Token。

不要在组件里直接写：

```css
color: #1d1d1f;
```

应该：

```css
color: var(--text-primary);
```

Apple HIG同样强调使用能够适应不同背景、外观模式和辅助功能设置的语义颜色思路。

---

# 20. 最终视觉层级

整个产品严格保持四层：

```text
Level 0
页面 Background

Level 1
内容 Surface

Level 2
Navigation / Toolbar

Level 3
Modal / Sheet
```

禁止：

```text
Card
里面 Card
里面又一个 Card
里面还有按钮框
```

这是很多 AI 产品最容易出现的问题。

---

# 21. 产品整体气质

最终用户打开 EchoNote 时应该感觉：

不是：

**“一个 AI SaaS。”**

不是：

**“一个播客管理后台。”**

也不是：

**“网页版 ChatGPT。”**

而应该感觉：

> **这是 iPhone 上一本专门帮我整理听过内容的智能笔记本。**

因此设计优先级固定为：

**内容 > 排版 > 留白 > 交互 > 装饰。**

任何视觉元素如果不能帮助用户：

**记录、阅读、理解、搜索、整理**

就应该删除。