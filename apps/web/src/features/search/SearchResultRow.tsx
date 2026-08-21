import type { SearchResult } from '../../shared/api/client'
import { RowLink } from '../../shared/components/RowLink'
import { HighlightedText } from './HighlightedText'

function tabForKind(kind: SearchResult['document_type']): string {
  if (kind === 'note') return 'notes'
  if (kind === 'transcript') return 'transcript'
  return 'ai'
}

function time(ms?: number): string {
  if (ms === undefined) return ''
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

export function SearchResultRow({ item, query }: { item: SearchResult; query: string }) {
  const meta = item.speaker_name ? `${item.speaker_name} ${time(item.start_ms)}` : item.document_type === 'note' ? '笔记' : item.document_type === 'ai_artifact' ? 'AI 整理' : time(item.start_ms)
  return <RowLink to={`/episode/${item.episode_id}?tab=${tabForKind(item.document_type)}`} className="block px-3.5 py-3"><p className="text-caption text-ink-secondary">{item.podcast_title}<span aria-hidden className="mx-1.5">·</span><span className="text-ink-tertiary">{meta}</span></p><p className="mt-1.5 font-serif text-body-serif text-ink"><HighlightedText text={item.snippet} query={query} /></p></RowLink>
}
