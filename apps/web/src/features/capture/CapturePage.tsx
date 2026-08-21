import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/client'
import { enqueueCapture } from '../../shared/outbox/captureOutbox'
import { useCaptureOutbox } from '../../shared/outbox/useCaptureOutbox'
import { useCaptureStore } from '../../shared/store/capture'
import { useVisualViewportHeight } from '../../shared/hooks/useVisualViewportHeight'
import { ShowCover } from '../../shared/components/ShowCover'
import { Waveform } from '../../shared/components/Waveform'

export function CapturePage() {
  const navigate = useNavigate()
  const location = useLocation()
  const episodes = useQuery({ queryKey: ['episodes'], queryFn: api.listEpisodes })
  const storedEpisodeId = useCaptureStore((state) => state.episodeId)
  const draft = useCaptureStore((state) => state.draft)
  const setEpisodeId = useCaptureStore((state) => state.setEpisodeId)
  const setDraft = useCaptureStore((state) => state.setDraft)
  const clearDraft = useCaptureStore((state) => state.clearDraft)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const visualViewportHeight = useVisualViewportHeight()
  const items = episodes.data?.items ?? []
  const episode = items.find((item) => item.id === storedEpisodeId) ?? items[0]
  const episodeId = episode?.id ?? (/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(storedEpisodeId) ? storedEpisodeId : '')
  const pending = useCaptureOutbox(episodeId || undefined)

  useEffect(() => {
    if (episode && episode.id !== storedEpisodeId) setEpisodeId(episode.id)
  }, [episode, setEpisodeId, storedEpisodeId])

  useEffect(() => {
    const timer = window.setTimeout(() => textareaRef.current?.focus(), 60)
    return () => window.clearTimeout(timer)
  }, [])

  const close = () => location.key !== 'default' ? navigate(-1) : navigate('/library')

  const finish = async () => {
    const text = draft.trim()
    if (!text || !episodeId) return
    setBusy(true)
    setError('')
    try {
      await enqueueCapture(episodeId, text)
      clearDraft()
      navigate(`/episode/${episodeId}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法保存到本地待发送队列。')
      setBusy(false)
    }
  }

  const textareaMinHeight = visualViewportHeight ? Math.max(220, Math.round(visualViewportHeight * 0.52)) : undefined
  const showTitle = episode?.podcast?.title ?? (episodeId ? '上次选择的节目' : '随手记录')

  return (
    <div className="safe-top safe-bottom safe-sides flex app-viewport w-full flex-col bg-canvas text-ink">
      <div className="mx-auto flex app-viewport w-full max-w-app flex-1 flex-col">
        <header className="flex min-h-11 items-center justify-between px-2 pt-1">
          <button type="button" onClick={close} className="flex min-h-11 items-center rounded-md px-3 text-callout text-ink-secondary">取消</button>
          <button type="button" onClick={finish} disabled={busy || !draft.trim() || !episodeId} className="flex min-h-11 items-center rounded-md px-3 text-callout font-medium text-accent disabled:opacity-40">{busy ? '保存中…' : '完成'}</button>
        </header>

        {items.length > 0 ? (
          <div className="px-4 pt-3">
            <div className="flex items-center gap-3">
              <ShowCover showTitle={showTitle} glyph={showTitle.slice(0, 1)} size="sm" />
              <div className="min-w-0 flex-1">
                <p className="text-headline text-ink">{showTitle}</p>
                <div className="mt-0.5 flex items-center gap-1.5 text-caption text-ink-tertiary"><Waveform bars={10} seed="capture" animated className="h-3 w-10 text-accent" />记录到所选节目</div>
              </div>
            </div>
            <select aria-label="选择节目" value={episode?.id ?? ''} onChange={(event) => setEpisodeId(event.target.value)} className="mt-3 min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-callout text-ink">
              {items.map((item) => <option key={item.id} value={item.id}>{item.title || '正在解析链接…'}</option>)}
            </select>
            {pending.length ? <p className="mt-2 text-caption text-ink-tertiary">此节目还有 {pending.length} 条本地记录待发送。</p> : null}
          </div>
        ) : episodeId ? (
          <p className="px-4 pt-6 text-body text-ink-secondary">当前离线，记录会保存到上次选择的节目并在联网后发送。</p>
        ) : episodes.isPending ? (
          <p className="px-4 pt-6 text-body text-ink-secondary">正在载入节目…</p>
        ) : (
          <p className="px-4 pt-6 text-body text-ink-secondary">资料库里还没有节目。请先返回节目页导入一期内容。</p>
        )}

        <textarea ref={textareaRef} value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') void finish() }} autoFocus autoComplete="off" spellCheck={false} aria-label="记录想法" placeholder="记录此刻的想法…" style={textareaMinHeight ? { minHeight: `${textareaMinHeight}px` } : undefined} className="capture-textarea-min w-full flex-1 resize-none bg-transparent px-4 pt-5 font-serif text-body-serif text-ink placeholder:font-sans placeholder:text-ink-tertiary focus:outline-none" />
        {error ? <p role="alert" className="px-4 pb-3 text-callout text-danger">{error}</p> : null}
      </div>
    </div>
  )
}
