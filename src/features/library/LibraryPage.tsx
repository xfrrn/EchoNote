import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useEpisodes } from '../../shared/mock/episodes'
import { EpisodeRow } from './EpisodeRow'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EchoMark } from '../../shared/components/EchoMark'
import { Waveform } from '../../shared/components/Waveform'
import { EmptyState } from '../../shared/components/EmptyState'
import { ImportSheet } from './ImportSheet'

export function LibraryPage() {
  const episodes = useEpisodes()
  const [importOpen, setImportOpen] = useState(false)

  return (
    <div>
      <header className="flex min-h-14 items-center justify-between px-4 pt-3">
        <h1 className="flex items-center gap-2 text-large-title text-ink">
          <span className="text-accent">
            <EchoMark size={26} />
          </span>
          EchoNote
        </h1>
        <button
          type="button"
          aria-label="导入节目"
          onClick={() => setImportOpen(true)}
          className="flex h-11 w-11 items-center justify-center rounded-full text-accent transition-colors duration-fast ease-ios active:bg-subtle"
        >
          <Plus size={26} strokeWidth={2} aria-hidden />
        </button>
      </header>

      {episodes.length === 0 ? (
        <EmptyState
          title="还没有节目"
          detail="点右上角的 ＋ 粘贴一期 Apple Podcasts 或小宇宙链接，EchoNote 会帮你转录、区分说话人，并把这期节目变成可以搜索和回看的笔记。"
        />
      ) : (
        <>
          <div className="flex items-center justify-between pr-4">
            <SectionLabel className="pt-4">最近</SectionLabel>
            <Waveform bars={18} seed="library" className="mt-4 h-4 w-24 text-ink-tertiary opacity-70" />
          </div>

          <section aria-label="最近节目">
            <div className="divide-y divide-hairline">
              {episodes.map((episode) => (
                <EpisodeRow key={episode.id} episode={episode} />
              ))}
            </div>
          </section>

          <p className="px-4 pt-6 text-caption text-ink-tertiary">
            {episodes.length} 个节目 · 听过的声音，都值得留下痕迹
          </p>
        </>
      )}

      <ImportSheet open={importOpen} onOpenChange={setImportOpen} />
    </div>
  )
}
