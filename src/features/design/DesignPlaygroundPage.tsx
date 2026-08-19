import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronLeft, Search } from 'lucide-react'
import { useTestMode } from '../../shared/store/test-mode'
import { SegmentedControl } from '../../shared/components/SegmentedControl'
import { Sheet } from '../../shared/components/Sheet'
import { EpisodeRow } from '../library/EpisodeRow'
import { NoteItem } from '../episode/NoteItem'
import { TranscriptSegmentItem } from '../episode/TranscriptSegmentItem'
import { SearchResultRow } from '../search/SearchResultRow'
import { BottomNav } from '../../app/layout/BottomNav'
import { getEpisodesSnapshot, getSpeaker } from '../../shared/mock/episodes'
import type { Note, SearchResultItem, TranscriptSegment } from '../../shared/types'

const colorTokens = [
  { name: '--bg-primary', className: 'bg-canvas', text: 'text-ink' },
  { name: '--bg-surface', className: 'bg-surface', text: 'text-ink' },
  { name: '--bg-secondary', className: 'bg-subtle', text: 'text-ink' },
  { name: '--text-primary', className: 'bg-ink', text: 'text-canvas' },
  { name: '--text-secondary', className: 'bg-ink-secondary', text: 'text-canvas' },
  { name: '--text-tertiary', className: 'bg-ink-tertiary', text: 'text-canvas' },
  { name: '--accent', className: 'bg-accent', text: 'text-on-accent' },
  { name: '--danger', className: 'bg-danger', text: 'text-on-accent' },
  { name: '--success', className: 'bg-success', text: 'text-on-accent' }
]

const typeScale = [
  { name: 'Large Title', className: 'text-large-title' },
  { name: 'Title 1', className: 'text-title-1' },
  { name: 'Title 2', className: 'text-title-2' },
  { name: 'Headline', className: 'text-headline' },
  { name: 'Body', className: 'text-body' },
  { name: 'Callout', className: 'text-callout' },
  { name: 'Subheadline', className: 'text-subheadline' },
  { name: 'Caption', className: 'text-caption' }
]

const sampleNote: Note = {
  id: 'design-note',
  createdAt: '19:45',
  text: '260 个步骤这个案例值得重点整理'
}

const sampleSegment: TranscriptSegment = {
  id: 'design-transcript',
  speakerId: 'peng',
  timestamp: '00:32:18',
  text: '其实我们当时遇到的问题是，企业的软件系统已经非常复杂，AI 真正进入企业之后，最难的不是模型能力，而是它能不能被业务现场真正理解。'
}

const sampleSearchResult: SearchResultItem = {
  kind: 'transcript',
  episodeId: 'e1',
  episodeTitle: 'E248｜一个“催发货”AI要跑通260步，和阿里瓴羊朋新宇聊聊中国式FDE',
  showTitle: '硅谷101',
  snippet: 'FDE 真正进入企业以后，需要处理的不是单个模型调用，而是围绕流程、权限、异常和人的一整套工程。',
  meta: '朋新宇 · 00:32:18'
}

function PlaygroundTitle({ children }: { children: string }) {
  return <h2 className="mt-8 px-4 text-title-2 text-ink">{children}</h2>
}

export function DesignPlaygroundPage() {
  const theme = useTestMode((state) => state.theme)
  const setTheme = useTestMode((state) => state.setTheme)
  const [segmentValue, setSegmentValue] = useState('notes')
  const [sheetOpen, setSheetOpen] = useState(false)
  const episode = getEpisodesSnapshot()[0]
  const speaker = getSpeaker(sampleSegment.speakerId)

  return (
    <div className="safe-top safe-sides app-viewport w-full bg-canvas text-ink">
      <div className="mx-auto w-full max-w-app pb-28">
        <header className="flex min-h-11 items-center gap-0.5 px-2 pt-3">
          <Link to="/mine" className="flex min-h-11 items-center gap-0.5 pl-1 pr-3 text-accent">
            <ChevronLeft size={24} strokeWidth={2.2} aria-hidden />
            <span className="text-headline">我的</span>
          </Link>
        </header>

        <div className="px-4">
          <h1 className="text-large-title text-ink">Design Playground</h1>
          <p className="mt-2 text-body text-ink-secondary">
            开发测试页，用于集中检查 Token、排版与组件层级。此页面不是正式产品页面。
          </p>
        </div>

        <PlaygroundTitle>Typography</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          {typeScale.map((item) => (
            <div key={item.name} className="flex items-baseline justify-between gap-4 px-4 py-3">
              <span className="text-caption text-ink-tertiary">{item.name}</span>
              <span className={item.className}>长文本阅读 Long-form reading</span>
            </div>
          ))}
        </div>

        <PlaygroundTitle>Colors</PlaygroundTitle>
        <div className="mt-2 grid grid-cols-3 gap-3 px-4">
          {colorTokens.map((color) => (
            <div key={color.name}>
              <div className={`flex h-12 items-end rounded-md p-2 ${color.className}`}>
                <span className={`text-caption ${color.text}`}>Aa</span>
              </div>
              <p className="mt-1 truncate text-caption text-ink-secondary">{color.name}</p>
            </div>
          ))}
        </div>
        <p className="px-4 pt-2 text-caption text-ink-tertiary">
          组件颜色全部来自语义 Token；切换页面底部的深浅色可验证 Dark Mode。
        </p>

        <PlaygroundTitle>Buttons / Inputs</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          <div className="flex flex-wrap items-center gap-3 px-4 py-4">
            <button
              type="button"
              className="flex min-h-11 items-center rounded-md bg-accent px-4 text-callout font-medium text-on-accent transition-colors duration-fast active:bg-accent"
            >
              主要操作
            </button>
            <button
              type="button"
              className="flex min-h-11 items-center rounded-md px-3 text-callout text-accent transition-colors duration-fast active:bg-subtle"
            >
              次要操作
            </button>
            <button
              type="button"
              className="flex min-h-11 items-center rounded-md px-3 text-callout text-ink-secondary transition-colors duration-fast active:bg-subtle"
            >
              安静操作
            </button>
          </div>
          <div className="flex items-center gap-2 px-4 py-4">
            <Search size={20} strokeWidth={1.8} className="text-ink-tertiary" aria-hidden />
            <input
              type="text"
              placeholder="搜索播客、内容和笔记"
              className="h-11 min-w-0 flex-1 rounded-md bg-subtle px-3 text-callout text-ink placeholder:text-ink-tertiary focus:outline-none"
            />
          </div>
        </div>

        <PlaygroundTitle>Segmented Control</PlaygroundTitle>
        <div className="mt-2 px-4">
          <SegmentedControl
            ariaLabel="Playground 分段控件"
            value={segmentValue}
            onChange={setSegmentValue}
            options={[
              { value: 'notes', label: '笔记' },
              { value: 'transcript', label: 'Transcript' },
              { value: 'ai', label: 'AI' }
            ]}
          />
        </div>

        <PlaygroundTitle>Episode Row</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">{episode ? <EpisodeRow episode={episode} /> : null}</div>

        <PlaygroundTitle>Note Item</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          <NoteItem note={sampleNote} />
        </div>

        <PlaygroundTitle>Transcript Segment</PlaygroundTitle>
        <div className="mt-2">
          <p className="px-4 pb-2 text-caption text-ink-tertiary">
            当前 Speaker：{speaker.name}；Timestamp 13px / Tertiary，正文 17px / 1.65。
          </p>
          <div className="divide-y divide-hairline">
            <TranscriptSegmentItem segment={sampleSegment} />
          </div>
        </div>

        <PlaygroundTitle>Search Result</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          <SearchResultRow item={sampleSearchResult} query="FDE" />
        </div>

        <PlaygroundTitle>Sheet</PlaygroundTitle>
        <div className="mt-2 px-4">
          <button
            type="button"
            onClick={() => setSheetOpen(true)}
            className="glass-surface flex min-h-11 w-full items-center rounded-md px-4 text-callout text-ink shadow-control transition-colors duration-fast active:text-ink-secondary"
          >
            打开 Sheet 示例
          </button>
        </div>

        <PlaygroundTitle>Dark Mode</PlaygroundTitle>
        <div className="mt-2 px-4">
          <SegmentedControl
            ariaLabel="深色模式"
            value={theme}
            onChange={setTheme}
            options={[
              { value: 'light', label: 'Light' },
              { value: 'dark', label: 'Dark' },
              { value: 'system', label: 'System' }
            ]}
          />
        </div>

        <PlaygroundTitle>Bottom Navigation</PlaygroundTitle>
        <p className="px-4 pt-2 text-caption text-ink-tertiary">
          当前页面底部显示的是真实 Bottom Navigation，可直接点击验证 Glass、Safe Area 与 44px 点击区。
        </p>
      </div>

      <BottomNav />

      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="Sheet 示例" description="演示底部 Sheet 的层级、Glass 与 Safe Area">
        <div className="px-4">
          <p className="text-body text-ink">
            这是 Control Layer 的 Sheet。内容区域不使用 Glass；关闭按钮和底部安全区都可以在此验证。
          </p>
          <button
            type="button"
            onClick={() => setSheetOpen(false)}
            className="mt-4 flex min-h-11 w-full items-center justify-center rounded-md bg-accent text-callout font-medium text-on-accent"
          >
            关闭
          </button>
        </div>
      </Sheet>
    </div>
  )
}
