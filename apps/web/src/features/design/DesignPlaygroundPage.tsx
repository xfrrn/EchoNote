import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronLeft, Search } from 'lucide-react'
import { useThemeStore } from '../../shared/store/theme'
import { SegmentedControl } from '../../shared/components/SegmentedControl'
import { Sheet } from '../../shared/components/Sheet'
import { EpisodeRow } from '../library/EpisodeRow'
import { NoteItem } from '../episode/NoteItem'
import { TranscriptSegmentItem } from '../episode/TranscriptSegmentItem'
import { SearchResultRow } from '../search/SearchResultRow'
import { BottomNav } from '../../app/layout/BottomNav'
import { ShowCover } from '../../shared/components/ShowCover'
import { EchoMark } from '../../shared/components/EchoMark'
import { Waveform } from '../../shared/components/Waveform'
import { getEpisodesSnapshot } from '../../shared/mock/episodes'
import type { EpisodeSummary, Note, SearchResult, TranscriptSegment, TranscriptSpeaker } from '../../shared/api/client'

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
  episode_id: 'e1',
  client_note_id: 'design-client-note',
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  content: '260 个步骤这个案例值得重点整理'
}

const sampleSegment: TranscriptSegment = {
  id: 'design-transcript',
  speaker_id: 'peng', sequence: 1, start_ms: 1_938_000, end_ms: 1_945_000,
  text: '其实我们当时遇到的问题是，企业的软件系统已经非常复杂，AI 真正进入企业之后，最难的不是模型能力，而是它能不能被业务现场真正理解。',
  words: [], source_chunk_id: 'design-chunk'
}

const sampleSpeaker: TranscriptSpeaker = { id: 'peng', stable_key: 'peng', display_name: '朋新宇', role: 'guest', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }

const sampleSearchResult: SearchResult = {
  id: 'design-search', document_type: 'transcript', source_id: sampleSegment.id, episode_id: 'e1',
  episode_title: 'E248｜一个“催发货”AI要跑通260步，和阿里瓴羊朋新宇聊聊中国式FDE',
  podcast_title: '硅谷101',
  snippet: 'FDE 真正进入企业以后，需要处理的不是单个模型调用，而是围绕流程、权限、异常和人的一整套工程。',
  speaker_name: '朋新宇', start_ms: 1_938_000, score: 1
}

function PlaygroundTitle({ children }: { children: string }) {
  return <h2 className="mt-8 px-4 text-title-2 text-ink">{children}</h2>
}

export function DesignPlaygroundPage() {
  const theme = useThemeStore((state) => state.theme)
  const setTheme = useThemeStore((state) => state.setTheme)
  const [segmentValue, setSegmentValue] = useState('notes')
  const [sheetOpen, setSheetOpen] = useState(false)
  const mockEpisode = getEpisodesSnapshot()[0]
  const episode: EpisodeSummary | undefined = mockEpisode ? {
    id: mockEpisode.id, title: mockEpisode.episodeTitle, duration_ms: mockEpisode.durationMin * 60_000,
    cover_url: '', resolve_status: 'completed', transcription_status: 'completed', ai_status: 'completed',
    source_count: 1, note_count: mockEpisode.notes.length, created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    podcast: { id: 'design-podcast', title: mockEpisode.showTitle, author: '', description: '', cover_url: '' }
  } : undefined

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

        <PlaygroundTitle>Brand / 母题</PlaygroundTitle>
        <div className="mt-2 flex items-center gap-6 px-4 py-2">
          <EchoMark size={32} className="text-accent" />
          <EchoMark size={32} className="text-accent" animated />
          <Waveform bars={24} seed="playground" className="h-6 w-32 text-ink-secondary" />
          <Waveform bars={24} seed="playground" animated className="h-6 w-32 text-accent" />
        </div>

        <PlaygroundTitle>Covers / 节目封面</PlaygroundTitle>
        <div className="mt-2 flex items-end gap-3 px-4">
          {getEpisodesSnapshot()
            .slice(0, 4)
            .map((ep) => (
              <div key={ep.id} className="flex flex-col items-center gap-1.5">
                <ShowCover showTitle={ep.showTitle} glyph={ep.coverLabel} size="lg" />
                <span className="max-w-16 truncate text-caption text-ink-tertiary">{ep.showTitle}</span>
              </div>
            ))}
        </div>

        <PlaygroundTitle>Brand · 回声母题与封面</PlaygroundTitle>
        <div className="mt-2 px-4">
          <div className="flex items-center gap-4">
            <EchoMark size={32} className="text-accent" />
            <EchoMark size={32} animated className="text-accent" />
            <Waveform bars={22} seed="playground" className="h-6 flex-1 text-ink-tertiary" />
            <Waveform bars={22} seed="playground" animated className="h-6 flex-1 text-accent" />
          </div>
          <div className="mt-4 flex items-end gap-3">
            <ShowCover showTitle="硅谷101" glyph="硅" size="xl" />
            <ShowCover showTitle="原点 The Origin" glyph="原" size="lg" />
            <ShowCover showTitle="声动早咖啡" glyph="声" size="md" />
            <ShowCover showTitle="晚点聊 LateTalk" glyph="晚" size="sm" />
          </div>
          <p className="mt-2 text-caption text-ink-tertiary">
            封面为程序化生成：暖调双色素场 + 衬线大字 + 声波 + 纸纹；未知节目按标题哈希取同色系。
          </p>
        </div>

        <PlaygroundTitle>Brand / 封面与母题</PlaygroundTitle>
        <div className="mt-2 flex items-center gap-4 px-4 py-2">
          <ShowCover showTitle="硅谷101" glyph="硅" size="xl" />
          <ShowCover showTitle="原点 The Origin" glyph="原" size="lg" />
          <ShowCover showTitle="声动早咖啡" glyph="声" size="md" />
          <ShowCover showTitle="晚点聊 LateTalk" glyph="晚" size="sm" />
        </div>
        <div className="mt-2 flex items-center gap-5 px-4 py-2">
          <span className="text-accent"><EchoMark size={30} /></span>
          <span className="text-accent"><EchoMark size={30} animated /></span>
          <Waveform bars={24} seed="playground" className="h-6 w-40 text-ink-secondary" />
          <Waveform bars={24} seed="playground-live" animated className="h-6 w-40 text-accent" />
        </div>
        <p className="px-4 pt-1 text-caption text-ink-tertiary">
          回声标（静态 / 扩散）与声波纹理（静态 / 起伏）。
        </p>

        <PlaygroundTitle>Brand / Covers</PlaygroundTitle>
        <div className="mt-2 flex items-center gap-4 px-4 py-2">
          <span className="text-accent">
            <EchoMark size={34} />
          </span>
          <span className="text-accent">
            <EchoMark size={34} animated />
          </span>
          <Waveform bars={22} seed="playground" className="h-6 w-32 text-ink-tertiary" />
          <Waveform bars={22} seed="playground-live" animated className="h-6 w-32 text-accent" />
        </div>
        <div className="mt-2 flex items-center gap-3 px-4">
          {getEpisodesSnapshot()
            .slice(0, 4)
            .map((ep) => (
              <ShowCover key={ep.id} showTitle={ep.showTitle} glyph={ep.coverLabel} size="lg" />
            ))}
        </div>
        <p className="px-4 pt-2 text-caption text-ink-tertiary">
          回声母题（静态 / 扩散）· 声波纹理（静态 / 起伏）· 四档节目封面同源不同色相。
        </p>

        <PlaygroundTitle>品牌 / 封面</PlaygroundTitle>
        <div className="mt-2 flex items-center gap-4 px-4">
          <ShowCover showTitle="硅谷101" glyph="硅" size="lg" />
          <ShowCover showTitle="原点 The Origin" glyph="原" size="lg" />
          <ShowCover showTitle="声动早咖啡" glyph="声" size="lg" />
          <ShowCover showTitle="晚点聊 LateTalk" glyph="晚" size="lg" />
        </div>
        <div className="mt-4 flex items-center gap-6 px-4">
          <span className="flex items-center gap-2 text-accent">
            <EchoMark size={28} />
            <EchoMark size={28} animated />
          </span>
          <Waveform bars={24} seed="playground" className="h-6 w-32 text-ink-secondary" />
          <Waveform bars={24} seed="playground" animated className="h-6 w-32 text-accent" />
        </div>
        <p className="mt-2 px-4 text-caption text-ink-tertiary">
          回声母题（涟漪 / 声波）与生成式封面：暖调双色素场 + 衬线大字 + 纸纹。
        </p>

        <PlaygroundTitle>Typography</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          {typeScale.map((item) => (
            <div key={item.name} className="flex items-baseline justify-between gap-4 px-4 py-3">
              <span className="text-caption text-ink-tertiary">{item.name}</span>
              <span className={item.className}>长文本阅读 Long-form reading</span>
            </div>
          ))}
          <div className="flex items-baseline justify-between gap-4 px-4 py-3">
            <span className="text-caption text-ink-tertiary">Body Serif</span>
            <span className="font-serif text-body-serif">长文本阅读 Long-form</span>
          </div>
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
        <div className="mt-2 divide-y divide-hairline">{episode ? <EpisodeRow episode={episode} deleting={false} onDelete={() => undefined} /> : null}</div>

        <PlaygroundTitle>Note Item</PlaygroundTitle>
        <div className="mt-2 divide-y divide-hairline">
          <NoteItem note={sampleNote} onSave={async () => undefined} onDelete={() => undefined} busy={false} />
        </div>

        <PlaygroundTitle>Transcript Segment</PlaygroundTitle>
        <div className="mt-2">
          <p className="px-4 pb-2 text-caption text-ink-tertiary">
            当前 Speaker：{sampleSpeaker.display_name}；正文衬线 17px / 1.78，署名用色点区分说话人。
          </p>
          <div>
            <TranscriptSegmentItem segment={sampleSegment} speaker={sampleSpeaker} showSpeaker />
            <TranscriptSegmentItem
              segment={{ ...sampleSegment, id: 'design-transcript-2', text: '同一位说话人的连续段落，不重复署名，只靠留白分段。' }} speaker={sampleSpeaker}
              showSpeaker={false}
            />
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
