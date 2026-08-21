import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../shared/api/client'
import { EpisodeRow } from './EpisodeRow'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EchoMark } from '../../shared/components/EchoMark'
import { EmptyState } from '../../shared/components/EmptyState'
import { InsetGroup } from '../../shared/components/InsetGroup'
import { ImportSheet } from './ImportSheet'

export function LibraryPage() {
  const queryClient = useQueryClient()
  const [importOpen, setImportOpen] = useState(false)
  const episodes = useQuery({ queryKey: ['episodes'], queryFn: api.listEpisodes })
  const remove = useMutation({
    mutationFn: api.deleteEpisode,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['episodes'] })
  })
  const items = episodes.data?.items ?? []

  const deleteEpisode = (id: string, title: string) => {
    if (window.confirm(`永久删除「${title}」及其笔记、Transcript 和 AI 内容？此操作无法撤销。`)) remove.mutate(id)
  }

  return (
    <div>
      <header className="flex min-h-14 items-center justify-between px-4 pt-3">
        <h1 className="flex items-center gap-2 text-large-title text-ink"><span className="text-accent"><EchoMark size={26} /></span>EchoNote</h1>
        <button type="button" aria-label="导入节目" onClick={() => setImportOpen(true)} className="flex h-11 w-11 items-center justify-center rounded-full text-accent transition-colors duration-fast ease-ios active:bg-subtle"><Plus size={26} strokeWidth={2} aria-hidden /></button>
      </header>

      {episodes.isPending ? (
        <p className="px-4 py-12 text-body text-ink-secondary">正在载入节目…</p>
      ) : episodes.isError ? (
        <EmptyState title="无法载入节目" detail={episodes.error instanceof Error ? episodes.error.message : '请检查网络后重试。'} />
      ) : items.length === 0 ? (
        <EmptyState title="还没有节目" detail="点右上角的 ＋ 粘贴一期 Apple Podcasts、小宇宙或音频链接。" />
      ) : (
        <>
          <SectionLabel className="pt-4">最近</SectionLabel>
          <section aria-label="最近节目" className="px-4"><InsetGroup>{items.map((episode) => <EpisodeRow key={episode.id} episode={episode} deleting={remove.isPending && remove.variables === episode.id} onDelete={deleteEpisode} />)}</InsetGroup></section>
          <p className="px-4 pt-4 text-caption text-ink-tertiary">{episodes.data?.total ?? items.length} 个节目 · 听过的声音，都值得留下痕迹</p>
        </>
      )}
      {remove.isError ? <p role="alert" className="px-4 pt-4 text-callout text-danger">{remove.error instanceof Error ? remove.error.message : '删除失败。'}</p> : null}
      <ImportSheet open={importOpen} onOpenChange={setImportOpen} />
    </div>
  )
}
