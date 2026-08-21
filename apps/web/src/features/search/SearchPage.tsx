import { useDeferredValue, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Search, X } from 'lucide-react'
import { api, type SearchResult } from '../../shared/api/client'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EmptyState } from '../../shared/components/EmptyState'
import { InsetGroup } from '../../shared/components/InsetGroup'
import { SearchResultRow } from './SearchResultRow'

const order: SearchResult['document_type'][] = ['note', 'transcript', 'ai_artifact']
const labels: Record<SearchResult['document_type'], string> = { note: '我的笔记', transcript: 'Transcript', ai_artifact: 'AI 整理' }

export function SearchPage() {
  const [query, setQuery] = useState('')
  const deferred = useDeferredValue(query.trim())
  const search = useQuery({ queryKey: ['search', deferred], queryFn: () => api.search(deferred), enabled: deferred.length >= 2 })
  const groups = useMemo(() => order.map((kind) => ({ kind, items: search.data?.items.filter((item) => item.document_type === kind) ?? [] })).filter((group) => group.items.length), [search.data])
  const hasQuery = deferred.length >= 2

  return (
    <div>
      <header className="px-4 pt-4">
        <h1 className="text-large-title text-ink">搜索</h1>
        <div className="mt-4 flex min-h-12 items-center gap-2 rounded-md bg-subtle px-3"><Search size={20} strokeWidth={1.8} className="shrink-0 text-ink-tertiary" aria-hidden /><input type="search" enterKeyHint="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 Transcript、笔记和 AI 整理" className="min-h-11 min-w-0 flex-1 bg-transparent text-callout text-ink placeholder:text-ink-tertiary focus:outline-none" />{query ? <button type="button" aria-label="清空搜索" onClick={() => setQuery('')} className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-ink-tertiary"><X size={18} aria-hidden /></button> : null}</div>
      </header>

      {!hasQuery ? <p className="px-4 pt-6 text-body text-ink-secondary">输入至少两个字符，搜索会覆盖你的笔记、Transcript 和 AI 整理。</p> : search.isPending ? <p className="px-4 py-10 text-body text-ink-secondary">正在搜索…</p> : search.isError ? <EmptyState title="搜索失败" detail={search.error instanceof Error ? search.error.message : '请稍后重试。'} /> : groups.length === 0 ? <EmptyState title={`没有找到与「${deferred}」相关的内容`} detail="换一个更短的关键词试试。" /> : <div className="pb-4">{groups.map((group) => <section key={group.kind} aria-label={labels[group.kind]}><SectionLabel className="pt-5">{labels[group.kind]}</SectionLabel><div className="px-4"><InsetGroup>{group.items.map((item) => <SearchResultRow key={item.id} item={item} query={deferred} />)}</InsetGroup></div></section>)}</div>}
    </div>
  )
}
