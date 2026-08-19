import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useCaptureStore, currentTimeLabel } from '../../shared/store/capture'
import { useVisualViewportHeight } from '../../shared/hooks/useVisualViewportHeight'
import { useEpisode } from '../../shared/mock/episodes'
import { ShowCover } from '../../shared/components/ShowCover'
import { Waveform } from '../../shared/components/Waveform'

export function CapturePage() {
  const navigate = useNavigate()
  const location = useLocation()
  const episodeId = useCaptureStore((state) => state.episodeId)
  const draft = useCaptureStore((state) => state.draft)
  const setDraft = useCaptureStore((state) => state.setDraft)
  const addNote = useCaptureStore((state) => state.addNote)
  const clearDraft = useCaptureStore((state) => state.clearDraft)
  const episode = useEpisode(episodeId)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const visualViewportHeight = useVisualViewportHeight()

  useEffect(() => {
    const timer = window.setTimeout(() => {
      textareaRef.current?.focus()
    }, 60)
    return () => window.clearTimeout(timer)
  }, [])

  const contextCode =
    episode?.episodeTitle.includes('｜') === true
      ? episode.episodeTitle.split('｜')[0]
      : (episode?.episodeTitle ?? '')

  const close = () => {
    if (location.key !== 'default') navigate(-1)
    else navigate('/library')
  }

  const finish = () => {
    const text = draft.trim()
    if (!text) {
      close()
      return
    }

    addNote(episodeId, {
      id: `local-${Date.now()}`,
      createdAt: currentTimeLabel(),
      text
    })
    clearDraft()
    navigate(`/episode/${episodeId}`)
  }

  const textareaMinHeight = visualViewportHeight
    ? Math.max(220, Math.round(visualViewportHeight * 0.52))
    : undefined

  return (
    <div className="safe-top safe-bottom safe-sides flex app-viewport w-full flex-col bg-canvas text-ink">
      <div className="mx-auto flex app-viewport w-full max-w-app flex-1 flex-col">
        <header className="flex min-h-11 items-center justify-between px-2 pt-1">
          <button
            type="button"
            onClick={close}
            className="flex min-h-11 items-center rounded-md px-3 text-callout text-ink-secondary transition-colors duration-fast ease-ios active:text-ink"
          >
            取消
          </button>
          <button
            type="button"
            onClick={finish}
            disabled={!draft.trim()}
            className="flex min-h-11 items-center rounded-md px-3 text-callout font-medium text-accent transition-opacity duration-fast ease-ios disabled:opacity-40"
          >
            完成
          </button>
        </header>

        <div className="flex items-center gap-3 px-4 pt-3">
          {episode ? (
            <ShowCover showTitle={episode.showTitle} glyph={episode.coverLabel} size="sm" />
          ) : null}
          <div className="min-w-0">
            <p className="text-headline text-ink">{episode?.showTitle ?? '随手记录'}</p>
            <p className="mt-0.5 flex items-center gap-1.5 text-caption text-ink-tertiary">
              <Waveform bars={10} seed="capture" animated className="h-3 w-10 text-accent" />
              {contextCode || '新想法'} · 正在记录
            </p>
          </div>
        </div>

        <textarea
          ref={textareaRef}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') finish()
          }}
          autoFocus
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="sentences"
          spellCheck={false}
          aria-label="记录想法"
          placeholder="记录此刻的想法…"
          style={textareaMinHeight ? { minHeight: `${textareaMinHeight}px` } : undefined}
          className="capture-textarea-min w-full flex-1 resize-none bg-transparent px-4 pt-5 font-serif text-body-serif text-ink placeholder:font-sans placeholder:text-ink-tertiary focus:outline-none"
        />
      </div>
    </div>
  )
}
