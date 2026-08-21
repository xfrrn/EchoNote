import type { components } from '@echonote/contracts'

type Schemas = components['schemas']

export type LoginRequest = Schemas['LoginRequest']
export type LoginResponse = Schemas['LoginResponse']
export type EpisodeSummary = Schemas['EpisodeSummary']
export type EpisodeDetail = Schemas['EpisodeDetail']
export type EpisodeListResponse = Schemas['EpisodeListResponse']
export type ImportResponse = Schemas['ImportResponse']
export type CreateCaptureRequest = Schemas['CreateCaptureRequest']
export type Note = Schemas['Note']
export type NoteListResponse = Schemas['NoteListResponse']
export type SearchResult = Schemas['SearchResult']
export type SearchResponse = Schemas['SearchResponse']
export type AIArtifact = Schemas['AIArtifact']
export type AIArtifactResult = Schemas['AIArtifactResult']
export type AICitation = Schemas['AICitation']
export type Conversation = Schemas['Conversation']
export type ExportContent = Schemas['ExportContent']
export type ExportMode = Schemas['ExportMode']
export type CreateExportRequest = Schemas['CreateExportRequest']
export type TranscriptionRun = Schemas['TranscriptionRun']
export type Transcript = Schemas['Transcript']
export type TranscriptSegment = Schemas['TranscriptSegment']
export type TranscriptSpeaker = Schemas['TranscriptSpeaker']

export const UNAUTHORIZED_EVENT = 'echonote:unauthorized'

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init: { method?: string; body?: unknown; signal?: AbortSignal } = {}): Promise<T> {
  const response = await fetch(path, {
    method: init.method,
    body: init.body === undefined ? undefined : JSON.stringify(init.body),
    headers: init.body === undefined ? undefined : { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    cache: 'no-store',
    signal: init.signal
  })
  if (!response.ok) throw await responseError(response)
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

async function responseError(response: Response): Promise<ApiError> {
  let body: Partial<Schemas['ErrorResponse']> = {}
  try {
    body = await response.json()
  } catch {
    // Some upstream failures do not have an API JSON body.
  }
  const error = new ApiError(response.status, body.code ?? 'HTTP_ERROR', body.message ?? `请求失败（${response.status}）`)
  if (response.status === 401) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
  return error
}

function encoded(value: string): string {
  return encodeURIComponent(value)
}

export const api = {
  login: (body: LoginRequest) => request<LoginResponse>('/api/v1/auth/login', { method: 'POST', body }),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
  me: () => request<LoginResponse>('/api/v1/me'),
  listEpisodes: () => request<EpisodeListResponse>('/api/v1/episodes?limit=100'),
  getEpisode: (id: string) => request<EpisodeDetail>(`/api/v1/episodes/${encoded(id)}`),
  deleteEpisode: (id: string) => request<void>(`/api/v1/episodes/${encoded(id)}`, { method: 'DELETE' }),
  createImport: (url: string) => request<ImportResponse>('/api/v1/imports', { method: 'POST', body: { url } }),
  getImport: (id: string) => request<ImportResponse>(`/api/v1/imports/${encoded(id)}`),
  createCapture: (body: CreateCaptureRequest) => request<Schemas['CaptureResponse']>('/api/v1/captures', { method: 'POST', body }),
  listNotes: (episodeId: string) => request<NoteListResponse>(`/api/v1/episodes/${encoded(episodeId)}/notes`),
  updateNote: (id: string, content: string) => request<Note>(`/api/v1/notes/${encoded(id)}`, { method: 'PATCH', body: { content } }),
  deleteNote: (id: string) => request<void>(`/api/v1/notes/${encoded(id)}`, { method: 'DELETE' }),
  search: (query: string, episodeId?: string) => {
    const params = new URLSearchParams({ q: query, scope: episodeId ? 'episode' : 'library', limit: '50' })
    if (episodeId) params.set('episode_id', episodeId)
    return request<SearchResponse>(`/api/v1/search?${params}`)
  },
  listArtifacts: (episodeId: string) => request<Schemas['AIArtifactList']>(`/api/v1/episodes/${encoded(episodeId)}/ai/artifacts`),
  requestArtifact: (episodeId: string) => request<AIArtifact>(`/api/v1/episodes/${encoded(episodeId)}/ai/artifacts`, { method: 'POST' }),
  createExport: (episodeId: string, body: CreateExportRequest) => request<ExportContent>(`/api/v1/episodes/${encoded(episodeId)}/exports`, { method: 'POST', body }),
  createConversation: (episodeId: string) => request<Conversation>('/api/v1/conversations', { method: 'POST', body: { scope: 'episode', episode_id: episodeId } }),
  getConversation: (id: string) => request<Conversation>(`/api/v1/conversations/${encoded(id)}`),
  createTranscription: (episodeId: string) => request<TranscriptionRun>(`/api/v1/episodes/${encoded(episodeId)}/transcriptions`, { method: 'POST', body: { profile: 'quality' } }),
  getTranscription: (id: string) => request<TranscriptionRun>(`/api/v1/transcriptions/${encoded(id)}`),
  retryTranscription: (id: string) => request<TranscriptionRun>(`/api/v1/transcriptions/${encoded(id)}/retry`, { method: 'POST' }),
  cancelTranscription: (id: string) => request<TranscriptionRun>(`/api/v1/transcriptions/${encoded(id)}/cancel`, { method: 'POST' }),
  getTranscript: (episodeId: string) => request<Transcript>(`/api/v1/episodes/${encoded(episodeId)}/transcript`),
  listSegments: (transcriptId: string, offset = 0) => request<Schemas['TranscriptSegmentList']>(`/api/v1/transcripts/${encoded(transcriptId)}/segments?limit=100&offset=${offset}`),
  renameSpeaker: (transcriptId: string, speakerId: string, displayName: string) => request<TranscriptSpeaker>(`/api/v1/transcripts/${encoded(transcriptId)}/speakers/${encoded(speakerId)}`, { method: 'PATCH', body: { display_name: displayName } }),
  mergeSpeakers: (transcriptId: string, sourceId: string, targetId: string) => request<TranscriptSpeaker>(`/api/v1/transcripts/${encoded(transcriptId)}/speakers/merge`, { method: 'POST', body: { source_speaker_id: sourceId, target_speaker_id: targetId } })
}

export type ChatStreamHandlers = {
  delta: (text: string) => void
  citation: (citation: AICitation) => void
  done: () => void
}

export async function streamConversationMessage(
  conversationId: string,
  content: string,
  handlers: ChatStreamHandlers,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch(`/api/v1/conversations/${encoded(conversationId)}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ client_message_id: crypto.randomUUID(), content }),
    credentials: 'same-origin',
    cache: 'no-store',
    signal
  })
  if (!response.ok) throw await responseError(response)
  if (!response.body) throw new ApiError(0, 'STREAM_UNAVAILABLE', '浏览器无法读取流式回答')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let completed = false

  const consume = (block: string) => {
    let event = 'message'
    const data: string[] = []
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
    }
    if (data.length === 0) return
    const payload = JSON.parse(data.join('\n')) as Record<string, unknown>
    if (event === 'delta') handlers.delta(String(payload.text ?? ''))
    else if (event === 'citation') handlers.citation(payload as AICitation)
    else if (event === 'done') {
      completed = true
      handlers.done()
    } else if (event === 'error') {
      throw new ApiError(500, String(payload.code ?? 'AI_STREAM_ERROR'), String(payload.message ?? 'AI 回答失败'))
    }
  }

  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, '\n')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      consume(buffer.slice(0, boundary))
      buffer = buffer.slice(boundary + 2)
      boundary = buffer.indexOf('\n\n')
    }
    if (done) break
  }
  if (buffer.trim()) consume(buffer)
  if (!completed) throw new ApiError(0, 'STREAM_INTERRUPTED', '连接中断，已保存的回答将在重试后恢复')
}
