import type { CreateCaptureRequest } from '../api/client'
import { api, ApiError } from '../api/client'

export const OUTBOX_EVENT = 'echonote:outbox-changed'

export type CaptureOutboxItem = CreateCaptureRequest & {
  state: 'pending' | 'blocked'
  attempts: number
  next_attempt_at: number
  last_error?: string
}

const DB_NAME = 'echonote'
const STORE_NAME = 'capture-outbox'
let databasePromise: Promise<IDBDatabase> | undefined
let replayPromise: Promise<void> | undefined
let replayTimer: number | undefined

function database(): Promise<IDBDatabase> {
  databasePromise ??= new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1)
    request.onupgradeneeded = () => request.result.createObjectStore(STORE_NAME, { keyPath: 'client_note_id' })
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
  return databasePromise
}

async function readAll(): Promise<CaptureOutboxItem[]> {
  const db = await database()
  return new Promise((resolve, reject) => {
    const request = db.transaction(STORE_NAME).objectStore(STORE_NAME).getAll()
    request.onsuccess = () => resolve((request.result as CaptureOutboxItem[]).sort((a, b) => a.created_at.localeCompare(b.created_at)))
    request.onerror = () => reject(request.error)
  })
}

async function write(method: 'add' | 'put' | 'delete' | 'clear', value?: CaptureOutboxItem | string): Promise<void> {
  const db = await database()
  await new Promise<void>((resolve, reject) => {
    const transaction = db.transaction(STORE_NAME, 'readwrite')
    const store = transaction.objectStore(STORE_NAME)
    if (method === 'clear') store.clear()
    else if (method === 'delete') store.delete(value as string)
    else store[method](value as CaptureOutboxItem)
    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error)
    transaction.onabort = () => reject(transaction.error)
  })
}

function changed(episodeId?: string): void {
  window.dispatchEvent(new CustomEvent(OUTBOX_EVENT, { detail: { episodeId } }))
}

export async function listCaptureOutbox(episodeId?: string): Promise<CaptureOutboxItem[]> {
  const items = await readAll()
  return episodeId ? items.filter((item) => item.episode_id === episodeId) : items
}

export async function enqueueCapture(episodeId: string, content: string): Promise<CaptureOutboxItem> {
  const item: CaptureOutboxItem = {
    client_note_id: crypto.randomUUID(),
    episode_id: episodeId,
    content,
    created_at: new Date().toISOString(),
    state: 'pending',
    attempts: 0,
    next_attempt_at: Date.now()
  }
  await write('add', item)
  changed(episodeId)
  void replayCaptureOutbox()
  return item
}

export async function retryCapture(clientNoteId: string): Promise<void> {
  const item = (await readAll()).find((candidate) => candidate.client_note_id === clientNoteId)
  if (!item) return
  await write('put', { ...item, state: 'pending', attempts: 0, next_attempt_at: Date.now(), last_error: undefined })
  changed(item.episode_id)
  await replayCaptureOutbox()
}

export async function discardCapture(clientNoteId: string): Promise<void> {
  const item = (await readAll()).find((candidate) => candidate.client_note_id === clientNoteId)
  await write('delete', clientNoteId)
  changed(item?.episode_id)
}

export async function clearCaptureOutbox(): Promise<void> {
  await write('clear')
  changed()
}

function scheduleReplay(delay: number): void {
  if (replayTimer !== undefined) window.clearTimeout(replayTimer)
  replayTimer = window.setTimeout(() => void replayCaptureOutbox(), delay)
}

export function replayCaptureOutbox(): Promise<void> {
  replayPromise ??= replay().finally(() => {
    replayPromise = undefined
  })
  return replayPromise
}

async function replay(): Promise<void> {
  if (!navigator.onLine) return
  for (const item of await readAll()) {
    if (item.state === 'blocked') return
    const delay = item.next_attempt_at - Date.now()
    if (delay > 0) {
      scheduleReplay(delay)
      return
    }
    try {
      await api.createCapture({
        client_note_id: item.client_note_id,
        episode_id: item.episode_id,
        episode_url: item.episode_url,
        content: item.content,
        created_at: item.created_at
      })
      await write('delete', item.client_note_id)
      changed(item.episode_id)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        await write('put', { ...item, state: 'blocked', last_error: `${error.code}: ${error.message}` })
        changed(item.episode_id)
        return
      }
      const attempts = item.attempts + 1
      const backoff = Math.min(300_000, 1000 * 2 ** Math.min(attempts - 1, 8))
      await write('put', { ...item, attempts, next_attempt_at: Date.now() + backoff, last_error: error instanceof Error ? error.message : '网络请求失败' })
      changed(item.episode_id)
      scheduleReplay(backoff)
      return
    }
  }
}
