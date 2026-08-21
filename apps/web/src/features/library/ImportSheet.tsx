import { useEffect, useRef, useState } from 'react'
import { ClipboardPaste, Link2, Loader2 } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Sheet } from '../../shared/components/Sheet'
import { EchoMark } from '../../shared/components/EchoMark'
import { api, type ImportResponse } from '../../shared/api/client'

interface ImportSheetProps { open: boolean; onOpenChange: (open: boolean) => void }

function terminal(value?: ImportResponse): boolean {
  return value?.status === 'succeeded' || value?.status === 'failed' || value?.status === 'canceled'
}

export function ImportSheet({ open, onOpenChange }: ImportSheetProps) {
  const queryClient = useQueryClient()
  const [url, setUrl] = useState('')
  const [importId, setImportId] = useState('')
  const [validation, setValidation] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const create = useMutation({ mutationFn: api.createImport, onSuccess: (item) => setImportId(item.id) })
  const status = useQuery({
    queryKey: ['import', importId], queryFn: () => api.getImport(importId), enabled: open && Boolean(importId),
    refetchInterval: (query) => terminal(query.state.data) ? false : 1000
  })

  useEffect(() => {
    if (status.data?.status === 'succeeded') {
      void queryClient.invalidateQueries({ queryKey: ['episodes'] })
      onOpenChange(false)
    }
  }, [onOpenChange, queryClient, status.data?.status])

  useEffect(() => {
    if (open) {
      const timer = window.setTimeout(() => inputRef.current?.focus(), 320)
      return () => window.clearTimeout(timer)
    }
    setUrl('')
    setImportId('')
    setValidation('')
    create.reset()
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const paste = async () => {
    try { setUrl((await navigator.clipboard.readText()).trim()) } catch { inputRef.current?.focus() }
  }

  const submit = () => {
    let parsed: URL
    try { parsed = new URL(url.trim()) } catch { setValidation('请输入有效的 HTTP 或 HTTPS 链接。'); return }
    if (!['http:', 'https:'].includes(parsed.protocol)) { setValidation('请输入有效的 HTTP 或 HTTPS 链接。'); return }
    setValidation('')
    create.mutate(parsed.toString())
  }

  const error = validation || status.data?.error?.message || (create.error instanceof Error ? create.error.message : '') || (status.error instanceof Error ? status.error.message : '')
  const busy = create.isPending || (Boolean(importId) && !terminal(status.data))

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="导入节目" description="粘贴播客或音频链接以解析">
      <div className="px-4 pb-2">
        <div className="flex items-start gap-3 pt-1"><span className="mt-0.5 text-accent"><EchoMark size={26} /></span><p className="text-body text-ink-secondary">粘贴一期节目的链接，EchoNote 会获取音频并加入资料库。</p></div>
        <div className="mt-5">
          <div className={`flex min-h-12 items-center gap-2 rounded-md border bg-surface px-3 ${error ? 'border-danger' : 'border-hairline focus-within:border-accent'}`}>
            <Link2 size={18} className="shrink-0 text-ink-tertiary" aria-hidden />
            <input ref={inputRef} type="url" inputMode="url" autoComplete="off" value={url} disabled={busy} onChange={(event) => { setUrl(event.target.value); setValidation('') }} onKeyDown={(event) => { if (event.key === 'Enter') submit() }} placeholder="Apple Podcasts / 小宇宙 / 音频链接" aria-label="节目链接" className="min-h-11 min-w-0 flex-1 bg-transparent text-callout text-ink placeholder:text-ink-tertiary focus:outline-none" />
            <button type="button" onClick={paste} disabled={busy} className="flex h-9 shrink-0 items-center gap-1 rounded-full bg-subtle px-3 text-caption-medium text-ink-secondary"><ClipboardPaste size={14} aria-hidden />粘贴</button>
          </div>
          <p className={`mt-2 text-caption ${error ? 'text-danger' : 'text-ink-tertiary'}`} role={error ? 'alert' : undefined}>{error || (status.data ? `当前阶段：${status.data.stage}` : '服务器会校验链接与音频来源。')}</p>
        </div>
        <button type="button" onClick={submit} disabled={!url.trim() || busy} className="mt-4 flex min-h-12 w-full items-center justify-center gap-2 rounded-md bg-accent text-body font-medium text-on-accent disabled:opacity-40">{busy ? <><Loader2 size={18} className="animate-spin" aria-hidden />正在处理…</> : '导入节目'}</button>
      </div>
    </Sheet>
  )
}
