import { useState } from 'react'
import type { Episode } from '../../shared/types'
import { useAiSummary } from '../../shared/mock/episodes'
import { EchoMark } from '../../shared/components/EchoMark'
import { EpisodeStatusState } from './EpisodeStatusState'
import { AiChatSheet } from './AiChatSheet'

function AiSectionTitle({ children }: { children: string }) {
  return (
    <h3 className="mb-3 mt-9 flex items-center gap-2 px-4 text-caption-medium tracking-[0.08em] text-ink-secondary first:mt-0">
      <span className="h-px w-4 bg-accent" aria-hidden />
      {children}
    </h3>
  )
}

export function AiTab({ episode }: { episode: Episode }) {
  const summary = useAiSummary(episode.id)
  const [chatOpen, setChatOpen] = useState(false)

  if (!episode.aiAvailable) {
    return <EpisodeStatusState episode={episode} kind="ai" />
  }

  return (
    <div className="pb-4">
      <div className="flex items-center gap-2 px-4 pt-4 text-caption text-ink-tertiary">
        <EchoMark size={14} className="text-accent" />
        <p>AI 整理 · 基于本期 Transcript 与你的 {episode.notes.length} 条笔记</p>
      </div>

      <section aria-label="一句话总结">
        <AiSectionTitle>一句话总结</AiSectionTitle>
        <p className="px-4 font-serif text-body-serif text-ink">{summary.oneLiner}</p>
      </section>

      <section aria-label="核心观点">
        <AiSectionTitle>核心观点</AiSectionTitle>
        <ol>
          {summary.corePoints.map((point, index) => (
            <li key={point} className="flex gap-3.5 px-4 py-2.5">
              <span className="w-5 shrink-0 pt-px text-right font-serif text-body-serif font-semibold text-accent">
                {index + 1}
              </span>
              <p className="font-serif text-body-serif text-ink">{point}</p>
            </li>
          ))}
        </ol>
      </section>

      <section aria-label="人物观点">
        <AiSectionTitle>人物观点</AiSectionTitle>
        <div>
          {summary.viewpoints.map((item) => (
            <div key={`${item.speaker}-${item.point}`} className="px-4 py-3.5">
              <p className="text-subheadline font-semibold text-ink">{item.speaker}</p>
              <p className="mt-1.5 font-serif text-body-serif text-ink">{item.point}</p>
            </div>
          ))}
        </div>
      </section>

      <section aria-label="值得回顾">
        <AiSectionTitle>值得回顾</AiSectionTitle>
        <div>
          {summary.worthReviewing.map((item) => (
            <div key={item.timestamp} className="px-4 py-3.5">
              <p className="text-caption tabular-nums text-ink-tertiary">{item.timestamp}</p>
              <p className="mt-1.5 font-serif text-body-serif font-medium text-ink">「{item.quote}」</p>
              <p className="mt-1.5 text-callout text-ink-secondary">{item.reason}</p>
            </div>
          ))}
        </div>
      </section>

      <section aria-label="结合我的笔记">
        <AiSectionTitle>结合我的笔记</AiSectionTitle>
        <div>
          {summary.noteConnections.map((item) => (
            <div key={item.note} className="px-4 py-3.5">
              <p className="font-serif text-body-serif text-ink">{item.note}</p>
              <p className="mt-2 border-l-2 border-accent-soft pl-3 text-callout text-ink-secondary">
                {item.insight}
              </p>
            </div>
          ))}
        </div>
      </section>

      <div className="h-24" />

      <div className="safe-sides fixed inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+76px)] z-30 mx-auto w-full max-w-app px-4">
        <button
          type="button"
          onClick={() => setChatOpen(true)}
          className="glass-surface flex min-h-12 w-full items-center gap-2.5 rounded-full px-4 text-callout text-ink-secondary shadow-control transition-colors duration-fast ease-ios active:text-ink"
        >
          <EchoMark size={17} className="shrink-0 text-accent" />
          问这期节目…
        </button>
      </div>

      <AiChatSheet open={chatOpen} onOpenChange={setChatOpen} episodeTitle={episode.episodeTitle} />
    </div>
  )
}
