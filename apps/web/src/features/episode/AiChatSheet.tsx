import { useEffect, useRef, useState } from 'react'
import { ArrowUp } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, streamConversationMessage, type AICitation } from '../../shared/api/client'
import { Sheet } from '../../shared/components/Sheet'

export function AiChatSheet({ open, onOpenChange, episodeId, episodeTitle }: { open: boolean; onOpenChange: (open: boolean) => void; episodeId: string; episodeTitle: string }) {
  const storageKey = `echonote.conversation.${episodeId}`
  const [conversationId, setConversationId] = useState(() => localStorage.getItem(storageKey) ?? '')
  const [query, setQuery] = useState('')
  const [streamed, setStreamed] = useState('')
  const [citations, setCitations] = useState<AICitation[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const controller = useRef<AbortController>()
  const queryClient = useQueryClient()
  const conversation = useQuery({ queryKey: ['conversation', conversationId], queryFn: () => api.getConversation(conversationId), enabled: open && Boolean(conversationId) })

  useEffect(() => {
    if (conversation.error instanceof ApiError && conversation.error.status === 404) {
      localStorage.removeItem(storageKey)
      setConversationId('')
    }
  }, [conversation.error, storageKey])

  useEffect(() => () => controller.current?.abort(), [])

  const ask = async () => {
    const content = query.trim()
    if (!content || busy) return
    let activeConversationId = conversationId
    setBusy(true)
    setError('')
    setStreamed('')
    setCitations([])
    setQuery('')
    try {
      if (!activeConversationId) {
        const created = await api.createConversation(episodeId)
        activeConversationId = created.id
        localStorage.setItem(storageKey, activeConversationId)
        setConversationId(activeConversationId)
      }
      controller.current = new AbortController()
      await streamConversationMessage(activeConversationId, content, {
        delta: (text) => setStreamed((value) => value + text),
        citation: (citation) => setCitations((value) => [...value, citation]),
        done: () => undefined
      }, controller.current.signal)
      await queryClient.invalidateQueries({ queryKey: ['conversation', activeConversationId] })
      setStreamed('')
      setCitations([])
    } catch (reason) {
      if (!(reason instanceof DOMException && reason.name === 'AbortError')) setError(reason instanceof Error ? reason.message : 'AI 回答失败。')
      setQuery(content)
      if (activeConversationId) void queryClient.invalidateQueries({ queryKey: ['conversation', activeConversationId] })
    } finally {
      setBusy(false)
    }
  }

  const messages = conversation.data?.messages ?? []
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="问这期节目" description={`基于 ${episodeTitle} 的 Transcript 与笔记`}>
      <div className="px-4">
        <div className="max-h-[48vh] space-y-5 overflow-y-auto pt-2">
          {messages.length === 0 && !streamed ? <p className="text-body text-ink-secondary">回答会保存到此会话，并附上可核对的 Transcript 或笔记引用。</p> : null}
          {messages.map((message) => <article key={message.id}><p className="text-caption-medium text-ink-secondary">{message.role === 'user' ? '你的问题' : 'AI 回答'}</p><p className="mt-1.5 whitespace-pre-wrap text-body text-ink">{message.content || (message.status === 'failed' ? message.error_message : '…')}</p>{message.citations.length ? <ul className="mt-2 space-y-1 text-caption text-ink-tertiary">{message.citations.map((citation, index) => <li key={`${citation.source_id}-${index}`}>引用：{citation.speaker_name ? `${citation.speaker_name} · ` : ''}{citation.excerpt}</li>)}</ul> : null}</article>)}
          {streamed ? <article><p className="text-caption-medium text-ink-secondary">AI 回答 · 生成中</p><p className="mt-1.5 whitespace-pre-wrap text-body text-ink">{streamed}</p>{citations.map((citation, index) => <p key={`${citation.source_id}-${index}`} className="mt-1 text-caption text-ink-tertiary">引用：{citation.excerpt}</p>)}</article> : null}
        </div>
        {error ? <p role="alert" className="mt-3 text-callout text-danger">{error}</p> : null}
        <div className="mt-6 flex items-end gap-2 border-t border-hairline pt-3">
          <input type="text" value={query} disabled={busy} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void ask() }} placeholder="问这期节目…" className="h-11 min-w-0 flex-1 rounded-md bg-subtle px-3 text-body text-ink placeholder:text-ink-tertiary" />
          <button type="button" aria-label="发送" onClick={() => void ask()} disabled={busy || !query.trim()} className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-accent text-on-accent disabled:opacity-40"><ArrowUp size={19} strokeWidth={2.2} aria-hidden /></button>
        </div>
      </div>
    </Sheet>
  )
}
