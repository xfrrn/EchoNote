import { useState } from 'react'
import type { Episode } from '../../shared/types'
import { useAiSummary } from '../../shared/mock/episodes'
import { EpisodeStatusState } from './EpisodeStatusState'
import { AiChatSheet } from './AiChatSheet'

function AiSectionTitle({ children }: { children: string }) {
  return <h3 className="mb-2 mt-8 px-4 text-caption-medium text-ink-secondary first:mt-0">{children}</h3>
}

export function AiTab({ episode }: { episode: Episode }) {
  const summary = useAiSummary(episode.id)
  const [chatOpen, setChatOpen] = useState(false)

  if (!episode.aiAvailable) {
    return <EpisodeStatusState episode={episode} kind="ai" />
  }

  return (
    <div className="pb-4">
      <div className="px-4 pt-4">
        <p className="text-caption-medium text-ink-secondary">AI 整理 · 基于本期 Transcript 与你的笔记</p>
      </div>

      <section aria-label="一句话总结">
        <AiSectionTitle>一句话总结</AiSectionTitle>
        <p className="px-4 text-body text-ink">{summary.oneLiner}</p>
      </section>

      <section aria-label="核心观点">
        <AiSectionTitle>核心观点</AiSectionTitle>
        <ol className="divide-y divide-hairline">
          {summary.corePoints.map((point, index) => (
            <li key={point} className="flex gap-3 px-4 py-3">
              <span className="w-5 shrink-0 text-callout font-medium text-accent">{index + 1}</span>
              <p className="text-body text-ink">{point}</p>
            </li>
          ))}
        </ol>
      </section>

      <section aria-label="人物观点">
        <AiSectionTitle>人物观点</AiSectionTitle>
        <div className="divide-y divide-hairline">
          {summary.viewpoints.map((item) => (
            <div key={`${item.speaker}-${item.point}`} className="px-4 py-4">
              <p className="text-subheadline font-semibold text-ink">{item.speaker}</p>
              <p className="mt-1 text-body text-ink">{item.point}</p>
            </div>
          ))}
        </div>
      </section>

      <section aria-label="值得回顾">
        <AiSectionTitle>值得回顾</AiSectionTitle>
        <div className="divide-y divide-hairline">
          {summary.worthReviewing.map((item) => (
            <div key={item.timestamp} className="px-4 py-4">
              <p className="text-caption text-ink-tertiary">{item.timestamp}</p>
              <p className="mt-1 text-body font-medium text-ink">{item.quote}</p>
              <p className="mt-1 text-body text-ink-secondary">{item.reason}</p>
            </div>
          ))}
        </div>
      </section>

      <section aria-label="结合我的笔记">
        <AiSectionTitle>结合我的笔记</AiSectionTitle>
        <div className="divide-y divide-hairline">
          {summary.noteConnections.map((item) => (
            <div key={item.note} className="px-4 py-4">
              <p className="text-body text-ink">{item.note}</p>
              <p className="mt-1.5 text-body text-ink-secondary">{item.insight}</p>
            </div>
          ))}
        </div>
      </section>

      <div className="h-24" />

      <div className="safe-sides fixed inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+72px)] z-30 mx-auto w-full max-w-app px-4">
        <button
          type="button"
          onClick={() => setChatOpen(true)}
          className="glass-surface flex min-h-11 w-full items-center rounded-md px-4 text-callout text-ink-secondary shadow-control transition-colors duration-fast ease-ios active:text-ink"
        >
          问这期节目…
        </button>
      </div>

      <AiChatSheet open={chatOpen} onOpenChange={setChatOpen} episodeTitle={episode.episodeTitle} />
    </div>
  )
}
