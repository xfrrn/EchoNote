import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { useEpisodes } from '../../shared/mock/episodes'
import { EpisodeRow } from './EpisodeRow'
import { SectionLabel } from '../../shared/components/SectionLabel'

export function LibraryPage() {
  const episodes = useEpisodes()
  const navigate = useNavigate()

  return (
    <div>
      <header className="flex min-h-14 items-center justify-between px-4 pt-3">
        <h1 className="text-large-title text-ink">EchoNote</h1>
        <button
          type="button"
          aria-label="记录想法"
          onClick={() => navigate('/capture')}
          className="flex h-11 w-11 items-center justify-center rounded-full text-accent transition-colors duration-fast ease-ios active:bg-subtle"
        >
          <Plus size={26} strokeWidth={2} aria-hidden />
        </button>
      </header>

      <SectionLabel className="pt-4">最近</SectionLabel>

      <section aria-label="最近节目">
        <div className="divide-y divide-hairline">
          {episodes.map((episode) => (
            <EpisodeRow key={episode.id} episode={episode} />
          ))}
        </div>
      </section>

      <p className="px-4 pt-6 text-caption text-ink-tertiary">
        {episodes.length} 个节目 · 本 Demo 使用本地 Mock 数据
      </p>
    </div>
  )
}
