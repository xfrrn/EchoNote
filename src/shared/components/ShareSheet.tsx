import { useMemo, useState } from 'react'
import { Check, SquareArrowOutUpRight, Copy } from 'lucide-react'
import type { AiSummary, Episode } from '../types'
import { Sheet } from './Sheet'
import { useTranscript, getSpeaker } from '../mock/episodes'
import {
  composeEpisodeMarkdown,
  defaultShareOptions,
  type ShareOptions
} from '../share/composeEpisodeMarkdown'

interface ShareSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  episode: Episode
  summary: AiSummary
}

const OPTION_META: { key: keyof ShareOptions; label: string; detail: string }[] = [
  { key: 'notes', label: '我的笔记', detail: '你在听节目时记下的想法' },
  { key: 'oneLiner', label: '一句话总结', detail: 'AI 提炼的本期主旨' },
  { key: 'corePoints', label: '核心观点', detail: 'AI 梳理的几条要点' },
  { key: 'worthReviewing', label: '值得回顾', detail: '值得重听的句子与时间' },
  { key: 'transcript', label: 'Transcript 节选', detail: '前几段正文，作为上下文' }
]

export function ShareSheet({ open, onOpenChange, episode, summary }: ShareSheetProps) {
  const [options, setOptions] = useState<ShareOptions>(defaultShareOptions)
  const [feedback, setFeedback] = useState<'idle' | 'shared' | 'copied'>('idle')
  const transcript = useTranscript(episode.id)

  const canShare = typeof navigator !== 'undefined' && typeof navigator.share === 'function'

  const excerpt = useMemo(
    () =>
      transcript.slice(0, 4).map((seg) => `${getSpeaker(seg.speakerId).name} ${seg.timestamp}：${seg.text}`),
    [transcript]
  )

  const selectedCount = OPTION_META.filter((o) => options[o.key]).length

  const toggle = (key: keyof ShareOptions) => {
    setFeedback('idle')
    setOptions((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const build = () => composeEpisodeMarkdown(episode, summary, options, excerpt)

  const share = async () => {
    const text = build()
    if (canShare) {
      try {
        await navigator.share({ title: episode.episodeTitle, text })
        setFeedback('shared')
        return
      } catch {
        // 用户取消分享，保持现状
        return
      }
    }
    await copy(text)
  }

  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setFeedback('copied')
    } catch {
      setFeedback('idle')
    }
  }

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title="分享到备忘录"
      description="把整理结果导出为一段文字，可分享到 Apple 备忘录"
    >
      <div className="px-4">
        <p className="pt-1 text-body text-ink-secondary">
          把这期节目整理成一条备忘录。选择要包含的内容：
        </p>

        <div className="mt-4 overflow-hidden rounded-lg bg-surface">
          <div className="divide-y divide-hairline">
            {OPTION_META.map((option) => {
              const checked = options[option.key]
              return (
                <button
                  key={option.key}
                  type="button"
                  role="checkbox"
                  aria-checked={checked}
                  onClick={() => toggle(option.key)}
                  className="flex w-full items-center justify-between gap-3 px-4 py-3.5 text-left transition-colors duration-fast ease-ios active:bg-subtle"
                >
                  <span className="min-w-0">
                    <span className="block text-body text-ink">{option.label}</span>
                    <span className="mt-0.5 block text-caption text-ink-tertiary">{option.detail}</span>
                  </span>
                  <span
                    className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border transition-colors duration-fast ease-ios ${
                      checked ? 'border-accent bg-accent text-on-accent' : 'border-hairline-strong text-transparent'
                    }`}
                  >
                    <Check size={15} strokeWidth={2.6} aria-hidden />
                  </span>
                </button>
              )
            })}
          </div>
        </div>

        <div className="mt-5 space-y-2.5 pb-2">
          <button
            type="button"
            onClick={share}
            disabled={selectedCount === 0}
            className="flex min-h-12 w-full items-center justify-center gap-2 rounded-md bg-accent text-body font-medium text-on-accent transition-opacity duration-fast ease-ios active:opacity-90 disabled:opacity-40"
          >
            <SquareArrowOutUpRight size={18} strokeWidth={2} aria-hidden />
            {canShare ? '分享到备忘录' : '复制到剪贴板'}
          </button>

          {canShare ? (
            <button
              type="button"
              onClick={() => copy(build())}
              disabled={selectedCount === 0}
              className="flex min-h-11 w-full items-center justify-center gap-2 rounded-md bg-subtle text-body text-ink transition-colors duration-fast ease-ios active:opacity-80 disabled:opacity-40"
            >
              <Copy size={17} strokeWidth={1.8} aria-hidden />
              复制全文
            </button>
          ) : null}

          <p className="pt-1 text-center text-caption text-ink-tertiary" role="status">
            {feedback === 'shared'
              ? '已调出系统分享，可直接选择「备忘录」。'
              : feedback === 'copied'
                ? '已复制。打开备忘录，长按粘贴即可。'
                : canShare
                  ? '在 iPhone 上可直接分享到「备忘录」App。'
                  : '当前环境不支持系统分享，将改为复制。'}
          </p>
        </div>
      </div>
    </Sheet>
  )
}
