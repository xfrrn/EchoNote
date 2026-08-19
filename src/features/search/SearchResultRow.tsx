import type { SearchResultItem } from '../../shared/types'
import { RowLink } from '../../shared/components/RowLink'
import { HighlightedText } from './HighlightedText'

function tabForKind(kind: SearchResultItem['kind']): string {
  if (kind === 'note') return 'notes'
  if (kind === 'transcript') return 'transcript'
  return 'ai'
}

export function SearchResultRow({ item, query }: { item: SearchResultItem; query: string }) {
  return (
    <RowLink
      to={`/episode/${item.episodeId}?tab=${tabForKind(item.kind)}`}
      className="block px-3.5 py-3"
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
    </RowLink>
  )
}
