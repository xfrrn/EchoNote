import { useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import { useSearchResults } from '../../shared/mock/episodes'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EmptyState } from '../../shared/components/EmptyState'
import { InsetGroup } from '../../shared/components/InsetGroup'
import { SearchResultRow } from './SearchResultRow'

const groupLabels = {
  note: '我的笔记',
  transcript: 'Transcript',
  ai: 'AI 整理'
} as const

export function SearchPage() {
  const [query, setQuery] = useState('')
  const results = useSearchResults(query)

  const groups = useMemo(() => {
    const order = ['note', 'transcript', 'ai'] as const
    return order
      .map((kind) => ({
        kind,
        items: results.filter((item) => item.kind === kind)
      }))
      .filter((group) => group.items.length > 0)
  }, [results])

  const hasQuery = query.trim().length >= 2

  return (
    <div>
      <header className="px-4 pt-4">
        <h1 className="text-large-title text-ink">搜索</h1>
        <div className="mt-4 flex min-h-12 items-center gap-2 rounded-md bg-subtle px-3">
          <Search size={20} strokeWidth={1.8} className="shrink-0 text-ink-tertiary" aria-hidden />
          <input
            type="text"
            enterKeyHint="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索播客、内容和笔记"
            className="min-h-11 min-w-0 flex-1 bg-transparent text-callout text-ink placeholder:text-ink-tertiary focus:outline-none"
          />
          {query ? (
            <button
              type="button"
              aria-label="清空搜索"
              onClick={() => setQuery('')}
              className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-ink-tertiary transition-colors duration-fast active:text-ink"
            >
              <X size={18} strokeWidth={1.8} aria-hidden />
            </button>
          ) : null}
        </div>
      </header>

      {!hasQuery ? (
        <p className="px-4 pt-6 text-body text-ink-secondary">
          搜索会覆盖我的笔记、Transcript 和 AI 整理。试试「FDE」「可信赖」或「260 步」。
        </p>
      ) : groups.length === 0 ? (
        <EmptyState title={`没有找到与「${query.trim()}」相关的内容`} detail="换一个更短的关键词试试，比如 FDE、Agent、转录。" />
      ) : (
        <div className="pb-4">
          {groups.map((group) => (
            <section key={group.kind} aria-label={groupLabels[group.kind]}>
              <SectionLabel className="pt-5">{groupLabels[group.kind]}</SectionLabel>
              <div className="px-4">
                <InsetGroup>
                  {group.items.map((item, index) => (
                    <SearchResultRow key={`${item.kind}-${item.meta}-${index}`} item={item} query={query} />
                  ))}
                </InsetGroup>
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
