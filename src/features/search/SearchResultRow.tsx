import { Link } from 'react-router-dom'
import type { SearchResultItem } from '../../shared/types'
import { HighlightedText } from './HighlightedText'

function tabForKind(kind: SearchResultItem['kind']): string {
  if (kind === 'note') return 'notes'
  if (kind === 'transcript') return 'transcript'
  return 'ai'
}

export function SearchResultRow({ item, query }: { item: SearchResultItem; query: string }) {
  return (
    <Link
      to={`/episode/${item.episodeId}?tab=${tabForKind(item.kind)}`}
      className="block px-3.5 py-3 transition-colors duration-fast ease-ios active:bg-subtle"
    >
      <p className="text-caption text-ink-secondary">
        {item.showTitle}
        <span aria-hidden className="mx-1.5">
          ·
        </span>
        <span className="text-ink-tertiary">{item.meta}</span>
      </p>
      <p className="mt-1.5 font-serif text-body-serif text-ink">
        <HighlightedText text={item.snippet} query={query} />
      </p>
    </Link>
  )
}
