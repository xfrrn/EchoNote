import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { useTestMode, type NotesDensity, type SpeakerCount, type TestModeState } from '../../shared/store/test-mode'
import { SegmentedControl, type SegmentedOption } from '../../shared/components/SegmentedControl'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EchoMark } from '../../shared/components/EchoMark'
import { statusLabels } from '../../shared/mock/episodes'

interface SettingRowProps {
  title: string
  detail: string
  children: React.ReactNode
}

function SettingRow({ title, detail, children }: SettingRowProps) {
  return (
    <div className="px-4 py-4">
      <div className="mb-2 flex items-baseline justify-between gap-3">
        <p className="text-body text-ink">{title}</p>
        <p className="text-caption text-ink-tertiary">{detail}</p>
      </div>
      {children}
    </div>
  )
}

export function MinePage() {
  const theme = useTestMode((state) => state.theme)
  const titleLength = useTestMode((state) => state.titleLength)
  const transcriptLength = useTestMode((state) => state.transcriptLength)
  const speakerCount = useTestMode((state) => state.speakerCount)
  const notesDensity = useTestMode((state) => state.notesDensity)
  const primaryStatus = useTestMode((state) => state.primaryStatus)
  const setTheme = useTestMode((state) => state.setTheme)
  const setTitleLength = useTestMode((state) => state.setTitleLength)
  const setTranscriptLength = useTestMode((state) => state.setTranscriptLength)
  const setSpeakerCount = useTestMode((state) => state.setSpeakerCount)
  const setNotesDensity = useTestMode((state) => state.setNotesDensity)
  const setPrimaryStatus = useTestMode((state) => state.setPrimaryStatus)

  const densityOptions: SegmentedOption<NotesDensity>[] = [
    { value: 'none', label: '无' },
    { value: 'few', label: '少量' },
    { value: 'many', label: '大量' }
  ]

  const statusOptions: SegmentedOption<TestModeState['primaryStatus']>[] = [
    { value: 'transcribed', label: '成功' },
    { value: 'transcribing', label: '转录中' },
    { value: 'failed', label: '失败' }
  ]

  return (
    <div>
      <header className="px-4 pt-4">
        <h1 className="text-large-title text-ink">我的</h1>
      </header>

      {/* 品牌区：回声标 + slogan */}
      <div className="mx-4 mt-4 flex items-center gap-3.5 rounded-lg bg-surface px-4 py-4">
        <span className="flex h-12 w-12 items-center justify-center rounded-lg bg-accent-soft text-accent">
          <EchoMark size={28} />
        </span>
        <div className="min-w-0">
          <p className="text-headline text-ink">EchoNote</p>
          <p className="mt-0.5 text-caption text-ink-secondary">把听过即忘的声音，变成能留下的知识。</p>
        </div>
      </div>

      <SectionLabel>外观</SectionLabel>
      <div className="divide-y divide-hairline">
        <SettingRow title="深浅色" detail="跟随系统或手动选择">
          <SegmentedControl
            ariaLabel="外观模式"
            value={theme}
            onChange={setTheme}
            options={[
              { value: 'light', label: '浅色' },
              { value: 'dark', label: '深色' },
              { value: 'system', label: '系统' }
            ]}
          />
        </SettingRow>
      </div>

      <SectionLabel>测试模式</SectionLabel>
      <div className="divide-y divide-hairline">
        <SettingRow title="标题长度" detail="验证超长中文标题">
          <SegmentedControl
            ariaLabel="标题长度"
            value={titleLength}
            onChange={setTitleLength}
            options={[
              { value: 'short', label: '短标题' },
              { value: 'long', label: '超长标题' }
            ]}
          />
        </SettingRow>
        <SettingRow title="Transcript 数量" detail="少量或可连续滚动的大量文本">
          <SegmentedControl
            ariaLabel="Transcript 数量"
            value={transcriptLength}
            onChange={setTranscriptLength}
            options={[
              { value: 'small', label: '少量' },
              { value: 'large', label: '大量' }
            ]}
          />
        </SettingRow>
        <SettingRow title="Speaker 数量" detail="验证文本层级是否稳定">
          <SegmentedControl<SpeakerCount>
            ariaLabel="Speaker 数量"
            value={speakerCount}
            onChange={setSpeakerCount}
            options={[
              { value: 1 as SpeakerCount, label: '1 位' },
              { value: 2 as SpeakerCount, label: '2 位' },
              { value: 4 as SpeakerCount, label: '4 位' }
            ]}
          />
        </SettingRow>
        <SettingRow title="笔记数量" detail="无笔记到大量笔记">
          <SegmentedControl
            ariaLabel="笔记数量"
            value={notesDensity}
            onChange={setNotesDensity}
            options={densityOptions}
          />
        </SettingRow>
        <SettingRow title="转录状态" detail="只改变「硅谷101」的模拟状态">
          <SegmentedControl
            ariaLabel="转录状态"
            value={primaryStatus}
            onChange={setPrimaryStatus}
            options={statusOptions}
          />
        </SettingRow>
        <SettingRow title="当前状态" detail="快速核对 Mock 数据">
          <p className="text-body text-ink-secondary">
            {titleLength === 'long' ? '超长标题' : '短标题'} · {transcriptLength === 'large' ? '大量' : '少量'}{' '}
            Transcript · {speakerCount} 位 Speaker ·{' '}
            {notesDensity === 'none' ? '无笔记' : notesDensity === 'few' ? '少量笔记' : '大量笔记'} ·{' '}
            {statusLabels[primaryStatus]}
          </p>
        </SettingRow>
      </div>

      <SectionLabel>开发</SectionLabel>
      <div className="divide-y divide-hairline">
        <Link
          to="/dev/design"
          className="flex min-h-11 items-center justify-between px-4 py-3 transition-colors duration-fast ease-ios active:bg-subtle"
        >
          <span className="text-body text-ink">Design Playground</span>
          <span className="flex items-center gap-1 text-ink-tertiary">
            <span className="text-caption">/dev/design</span>
            <ChevronRight size={18} strokeWidth={1.8} aria-hidden />
          </span>
        </Link>
        <div className="px-4 py-4">
          <p className="text-body text-ink">EchoNote v0.1</p>
          <p className="mt-1 text-caption text-ink-tertiary">
            高保真 PWA Demo · 全部内容为本地 Mock 数据，不连接任何真实服务。
          </p>
        </div>
      </div>
    </div>
  )
}
