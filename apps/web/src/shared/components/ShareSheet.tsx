import { useState } from 'react'
import { Check, Copy, SquareArrowOutUpRight } from 'lucide-react'
import type { CreateExportRequest, EpisodeDetail } from '../api/client'
import { api } from '../api/client'
import { Sheet } from './Sheet'

type Options = { notes: boolean; summary: boolean; points: boolean; review: boolean; transcript: boolean }
const defaults: Options = { notes: true, summary: true, points: true, review: true, transcript: false }
const meta: { key: keyof Options; label: string; detail: string }[] = [
  { key: 'notes', label: '我的笔记', detail: '你记录的想法' },
  { key: 'summary', label: '一句话总结', detail: 'AI 提炼的本期主旨' },
  { key: 'points', label: '核心观点', detail: 'AI 梳理的重点' },
  { key: 'review', label: '值得回顾', detail: '值得重听的片段' },
  { key: 'transcript', label: 'Transcript 节选', detail: '前几段正文作为上下文' }
]

export function ShareSheet({ open, onOpenChange, episode }: { open: boolean; onOpenChange: (open: boolean) => void; episode: EpisodeDetail }) {
  const [options, setOptions] = useState(defaults)
  const [feedback, setFeedback] = useState('')
  const [busy, setBusy] = useState(false)
  const canShare = typeof navigator.share === 'function'
  const selected = Object.values(options).filter(Boolean).length

  const build = () => api.createExport(episode.id, {
    mode: 'organized_note', include_user_notes: options.notes, include_summary: options.summary,
    include_key_points: options.points, include_worth_reviewing: options.review, include_transcript: options.transcript
  } satisfies CreateExportRequest)

  const copy = async () => {
    setBusy(true); setFeedback('')
    try { const output = await build(); await navigator.clipboard.writeText(output.text); setFeedback('已复制全文。') }
    catch (reason) { setFeedback(reason instanceof Error ? reason.message : '导出失败。') }
    finally { setBusy(false) }
  }

  const share = async () => {
    if (!canShare) { await copy(); return }
    setBusy(true); setFeedback('')
    try { const output = await build(); await navigator.share({ title: output.title, text: output.text }); setFeedback('已调出系统分享。') }
    catch (reason) { if (!(reason instanceof DOMException && reason.name === 'AbortError')) setFeedback(reason instanceof Error ? reason.message : '分享失败。') }
    finally { setBusy(false) }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="导出与分享" description="由服务器从同一份节目快照生成">
      <div className="px-4">
        <div className="mt-2 overflow-hidden rounded-lg bg-surface"><div className="divide-y divide-hairline">{meta.map((option) => { const checked = options[option.key]; return <button key={option.key} type="button" role="checkbox" aria-checked={checked} onClick={() => { setFeedback(''); setOptions((value) => ({ ...value, [option.key]: !value[option.key] })) }} className="flex w-full items-center justify-between gap-3 px-4 py-3.5 text-left active:bg-subtle"><span><span className="block text-body text-ink">{option.label}</span><span className="mt-0.5 block text-caption text-ink-tertiary">{option.detail}</span></span><span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border ${checked ? 'border-accent bg-accent text-on-accent' : 'border-hairline-strong text-transparent'}`}><Check size={15} strokeWidth={2.6} aria-hidden /></span></button> })}</div></div>
        <div className="mt-5 space-y-2.5 pb-2">
          <button type="button" onClick={() => void share()} disabled={!selected || busy} className="flex min-h-12 w-full items-center justify-center gap-2 rounded-md bg-accent text-body font-medium text-on-accent disabled:opacity-40"><SquareArrowOutUpRight size={18} aria-hidden />{canShare ? '系统分享' : '复制到剪贴板'}</button>
          {canShare ? <button type="button" onClick={() => void copy()} disabled={!selected || busy} className="flex min-h-11 w-full items-center justify-center gap-2 rounded-md bg-subtle text-body text-ink disabled:opacity-40"><Copy size={17} aria-hidden />复制全文</button> : null}
          <p role="status" className="pt-1 text-center text-caption text-ink-tertiary">{busy ? '正在生成导出内容…' : feedback || 'iPhone 上可直接选择「备忘录」。'}</p>
        </div>
      </div>
    </Sheet>
  )
}
