import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type EpisodeDetail } from '../../shared/api/client'
import { discardCapture, retryCapture } from '../../shared/outbox/captureOutbox'
import { useCaptureOutbox } from '../../shared/outbox/useCaptureOutbox'
import { useCaptureStore } from '../../shared/store/capture'
import { NoteItem } from './NoteItem'
import { EmptyState } from '../../shared/components/EmptyState'
import { InsetGroup } from '../../shared/components/InsetGroup'

export function NotesTab({ episode }: { episode: EpisodeDetail }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const setEpisodeId = useCaptureStore((state) => state.setEpisodeId)
  const notes = useQuery({ queryKey: ['notes', episode.id], queryFn: () => api.listNotes(episode.id) })
  const outbox = useCaptureOutbox(episode.id)
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['notes', episode.id] })
    void queryClient.invalidateQueries({ queryKey: ['episodes'] })
  }
  const update = useMutation({ mutationFn: ({ id, content }: { id: string; content: string }) => api.updateNote(id, content), onSuccess: refresh })
  const remove = useMutation({ mutationFn: api.deleteNote, onSuccess: refresh })

  const startCapture = () => {
    setEpisodeId(episode.id)
    navigate('/capture')
  }

  const empty = !notes.isPending && (notes.data?.items.length ?? 0) === 0 && outbox.length === 0
  return (
    <div className="pb-4">
      {outbox.length > 0 ? (
        <div className="px-4 pt-4">
          <p className="mb-2 text-caption-medium text-ink-secondary">本地待发送</p>
          <InsetGroup>
            {outbox.map((item) => (
              <article key={item.client_note_id} className="px-3.5 py-3">
                <p className="font-serif text-body-serif text-ink">{item.content}</p>
                <p className={`mt-1.5 text-caption ${item.state === 'blocked' ? 'text-danger' : 'text-ink-tertiary'}`}>{item.state === 'blocked' ? item.last_error ?? '需要手动处理' : navigator.onLine ? '等待发送…' : '离线保存，联网后自动发送'}</p>
                <div className="mt-2 flex gap-4">
                  {item.state === 'blocked' ? <button type="button" onClick={() => void retryCapture(item.client_note_id)} className="min-h-11 text-callout text-accent">重试</button> : null}
                  <button type="button" onClick={() => void discardCapture(item.client_note_id)} className="min-h-11 text-callout text-danger">删除本地记录</button>
                </div>
              </article>
            ))}
          </InsetGroup>
        </div>
      ) : null}

      {notes.isPending ? <p className="px-4 py-10 text-body text-ink-secondary">正在载入笔记…</p> : null}
      {notes.isError ? <p role="alert" className="px-4 py-6 text-body text-danger">{notes.error instanceof Error ? notes.error.message : '笔记载入失败。'}</p> : null}
      {remove.isError ? <p role="alert" className="px-4 py-3 text-body text-danger">{remove.error instanceof Error ? remove.error.message : '删除失败。'}</p> : null}
      {empty ? <EmptyState title="还没有笔记" detail="听节目时冒出的想法，可用底部「记录」先写入本地；离线时也不会丢失。" /> : null}
      {(notes.data?.items.length ?? 0) > 0 ? (
        <div className="px-4 pt-4"><InsetGroup>{notes.data!.items.map((note) => <NoteItem key={note.id} note={note} onSave={(content) => update.mutateAsync({ id: note.id, content })} onDelete={() => remove.mutate(note.id)} busy={(update.isPending && update.variables?.id === note.id) || (remove.isPending && remove.variables === note.id)} />)}</InsetGroup></div>
      ) : null}

      <div className="mt-2 px-4"><button type="button" onClick={startCapture} className="flex min-h-11 w-full items-center gap-2 rounded-md text-accent active:bg-subtle"><Plus size={19} strokeWidth={2} aria-hidden /><span className="text-callout">记录想法</span></button></div>
    </div>
  )
}
