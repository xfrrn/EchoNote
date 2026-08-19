import { useEffect, useRef, useState } from 'react'
import { ClipboardPaste, Link2, Loader2 } from 'lucide-react'
import { Sheet } from '../../shared/components/Sheet'
import { EchoMark } from '../../shared/components/EchoMark'
import { buildImportedEpisode } from '../../shared/mock/episodes'
import { useLibraryStore } from '../../shared/store/library'

interface ImportSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function looksLikePodcastUrl(value: string): boolean {
  const v = value.trim()
  if (!/^https?:\/\//i.test(v)) return false
  return /\./.test(v)
}

export function ImportSheet({ open, onOpenChange }: ImportSheetProps) {
  const addImported = useLibraryStore((s) => s.addImported)
  const [url, setUrl] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setError('')
      setBusy(false)
      const timer = window.setTimeout(() => inputRef.current?.focus(), 320)
      return () => window.clearTimeout(timer)
    }
    setUrl('')
    setError('')
    setBusy(false)
  }, [open])

  const paste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text) {
        setUrl(text.trim())
        setError('')
      }
    } catch {
      inputRef.current?.focus()
    }
  }

  const submit = () => {
    const value = url.trim()
    if (!looksLikePodcastUrl(value)) {
      setError('这看起来不是一个有效的节目链接，请检查后重试。')
      return
    }
    setBusy(true)
    // Demo：模拟「解析链接 → 加入转录队列」的短暂过程
    window.setTimeout(() => {
      addImported(buildImportedEpisode(value))
      setBusy(false)
      onOpenChange(false)
    }, 700)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="导入节目" description="粘贴播客链接以转录">
      <div className="px-4 pb-2">
        <div className="flex items-start gap-3 pt-1">
          <span className="mt-0.5 text-accent">
            <EchoMark size={26} />
          </span>
          <p className="text-body text-ink-secondary">
            粘贴一期节目的链接，EchoNote 会自动获取音频、转录并区分说话人。你照常在自己的播客 App 里听，这里负责帮你记住。
          </p>
        </div>

        <div className="mt-5">
          <div
            className={`flex min-h-12 items-center gap-2 rounded-md border bg-surface px-3 transition-colors duration-fast ease-ios ${
              error ? 'border-danger' : 'border-hairline focus-within:border-accent'
            }`}
          >
            <Link2 size={18} strokeWidth={1.8} className="shrink-0 text-ink-tertiary" aria-hidden />
            <input
              ref={inputRef}
              type="url"
              inputMode="url"
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="none"
              spellCheck={false}
              value={url}
              onChange={(event) => {
                setUrl(event.target.value)
                if (error) setError('')
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') submit()
              }}
              placeholder="粘贴 Apple Podcasts / 小宇宙链接"
              aria-label="节目链接"
              aria-invalid={Boolean(error)}
              className="min-h-11 min-w-0 flex-1 bg-transparent text-callout text-ink placeholder:text-ink-tertiary focus:outline-none"
            />
            <button
              type="button"
              onClick={paste}
              className="flex h-9 shrink-0 items-center gap-1 rounded-full bg-subtle px-3 text-caption-medium text-ink-secondary transition-colors duration-fast ease-ios active:opacity-75"
            >
              <ClipboardPaste size={14} strokeWidth={2} aria-hidden />
              粘贴
            </button>
          </div>
          <p className={`mt-2 text-caption ${error ? 'text-danger' : 'text-ink-tertiary'}`} role={error ? 'alert' : undefined}>
            {error || '支持 Apple Podcasts、小宇宙的节目或单集链接。'}
          </p>
        </div>

        <button
          type="button"
          onClick={submit}
          disabled={!url.trim() || busy}
          className="mt-4 flex min-h-12 w-full items-center justify-center gap-2 rounded-md bg-accent text-body font-medium text-on-accent transition-opacity duration-fast ease-ios active:opacity-90 disabled:opacity-40"
        >
          {busy ? (
            <>
              <Loader2 size={18} strokeWidth={2.2} className="animate-spin" aria-hidden />
              正在解析链接…
            </>
          ) : (
            '导入并转录'
          )}
        </button>
      </div>
    </Sheet>
  )
}
