import { randomUUID } from 'node:crypto'
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { createMcpExpressApp } from '@modelcontextprotocol/sdk/server/express.js'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js'
import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js'
import type { Request, Response } from 'express'
import * as z from 'zod/v4'
import { EchoNoteClient, parseSSE } from './api.js'

type Annotations = {
  readOnlyHint: boolean
  destructiveHint: boolean
  openWorldHint: boolean
}

const readOnly: Annotations = { readOnlyHint: true, destructiveHint: false, openWorldHint: false }
const write: Annotations = { readOnlyHint: false, destructiveHint: false, openWorldHint: false }
const externalWrite: Annotations = { ...write, openWorldHint: true }
const destructive: Annotations = { ...write, destructiveHint: true }
const uuid = z.uuid().describe('UUID returned by EchoNote')

function createServer(api: EchoNoteClient): McpServer {
  const server = new McpServer(
    { name: 'echonote', version: '0.1.0' },
    {
      instructions:
        'Use list_episodes or search_content to find stable IDs before calling ID-based tools. Never guess IDs. Confirm with the user before delete_episode, delete_note, or merge_transcript_speakers. Imports, transcription, summaries, and conversation answers may start asynchronous or paid external work; report returned status and IDs.',
    },
  )

  tool(server, 'get_service_status', 'Get service status', 'Check whether EchoNote and PostgreSQL are ready.', {}, readOnly, () =>
    api.json('/readyz'),
  )
  tool(
    server,
    'import_episode',
    'Import episode',
    'Import an Apple Podcasts episode, RSS feed, or direct audio URL into EchoNote.',
    { url: z.url().describe('Episode, feed, or direct audio URL') },
    externalWrite,
    ({ url }) => api.json('/api/v1/imports', { method: 'POST', body: { url } }),
  )
  tool(
    server,
    'get_import',
    'Get import',
    'Check an import job and obtain its episode ID when resolved.',
    { import_id: uuid },
    readOnly,
    ({ import_id }) => api.json(`/api/v1/imports/${encode(import_id)}`),
  )
  tool(
    server,
    'list_episodes',
    'List episodes',
    'List recently imported episodes in the EchoNote library.',
    {
      limit: z.int().min(1).max(100).default(50).describe('Page size'),
      offset: z.int().min(0).default(0).describe('Page offset'),
    },
    readOnly,
    ({ limit, offset }) => api.json(query('/api/v1/episodes', { limit, offset })),
  )
  tool(
    server,
    'get_episode',
    'Get episode',
    'Get one episode with podcast, source, and processing details.',
    { episode_id: uuid },
    readOnly,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}`),
  )
  tool(
    server,
    'delete_episode',
    'Delete episode',
    'Permanently delete an episode and its dependent EchoNote data.',
    { episode_id: uuid },
    destructive,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}`, { method: 'DELETE' }),
  )
  tool(
    server,
    'capture_note',
    'Capture note',
    'Save a note against an existing episode or a URL; URL capture also starts an import.',
    {
      content: z.string().trim().min(1).describe('Note text'),
      episode_id: uuid.optional(),
      episode_url: z.url().optional(),
      client_note_id: uuid.optional().describe('Stable idempotency key; generated when omitted'),
      created_at: z.iso.datetime().optional().describe('Client creation time; current time when omitted'),
    },
    externalWrite,
    ({ content, episode_id, episode_url, client_note_id, created_at }) => {
      if (!!episode_id === !!episode_url) throw new Error('Provide exactly one of episode_id and episode_url')
      return api.json('/api/v1/captures', {
        method: 'POST',
        body: {
          content,
          episode_id,
          episode_url,
          client_note_id: client_note_id ?? randomUUID(),
          created_at: created_at ?? new Date().toISOString(),
        },
      })
    },
  )
  tool(
    server,
    'list_episode_notes',
    'List episode notes',
    'List active notes for an episode, newest first.',
    { episode_id: uuid },
    readOnly,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}/notes`),
  )
  tool(
    server,
    'create_episode_note',
    'Create episode note',
    'Create an idempotent note for an existing episode.',
    {
      episode_id: uuid,
      content: z.string().trim().min(1).describe('Note text'),
      client_note_id: uuid.optional().describe('Stable idempotency key; generated when omitted'),
      created_at: z.iso.datetime().optional().describe('Client creation time; current time when omitted'),
    },
    write,
    ({ episode_id, content, client_note_id, created_at }) =>
      api.json(`/api/v1/episodes/${encode(episode_id)}/notes`, {
        method: 'POST',
        body: {
          content,
          client_note_id: client_note_id ?? randomUUID(),
          created_at: created_at ?? new Date().toISOString(),
        },
      }),
  )
  tool(
    server,
    'update_note',
    'Update note',
    'Replace the text of an existing note.',
    { note_id: uuid, content: z.string().trim().min(1).describe('Replacement note text') },
    write,
    ({ note_id, content }) => api.json(`/api/v1/notes/${encode(note_id)}`, { method: 'PATCH', body: { content } }),
  )
  tool(
    server,
    'delete_note',
    'Delete note',
    'Soft-delete a note that has no restore operation.',
    { note_id: uuid },
    destructive,
    ({ note_id }) => api.json(`/api/v1/notes/${encode(note_id)}`, { method: 'DELETE' }),
  )
  tool(
    server,
    'search_content',
    'Search EchoNote',
    'Search notes, transcripts, and generated summaries in the library or one episode.',
    {
      query: z.string().trim().min(2).max(500).describe('Search query'),
      scope: z.enum(['library', 'episode']).default('library'),
      episode_id: uuid.optional().describe('Required when scope is episode'),
      limit: z.int().min(1).max(50).default(20),
    },
    readOnly,
    ({ query: searchQuery, scope, episode_id, limit }) => {
      if (scope === 'episode' && !episode_id) throw new Error('episode_id is required when scope is episode')
      return api.json(query('/api/v1/search', { q: searchQuery, scope, episode_id, limit }))
    },
  )
  tool(
    server,
    'reindex_search',
    'Reindex search',
    'Queue a search-index rebuild for the library or one episode.',
    { scope: z.enum(['library', 'episode']), episode_id: uuid.optional().describe('Required when scope is episode') },
    write,
    ({ scope, episode_id }) => {
      if (scope === 'episode' && !episode_id) throw new Error('episode_id is required when scope is episode')
      return api.json('/api/v1/search/reindex', { method: 'POST', body: { scope, episode_id } })
    },
  )
  tool(
    server,
    'list_episode_ai_artifacts',
    'List episode summaries',
    'List current and historical generated summaries for an episode.',
    { episode_id: uuid },
    readOnly,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}/ai/artifacts`),
  )
  tool(
    server,
    'request_episode_summary',
    'Generate episode summary',
    'Return a cached episode summary or queue generation from the active transcript.',
    { episode_id: uuid },
    externalWrite,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}/ai/artifacts`, { method: 'POST' }),
  )
  tool(
    server,
    'export_episode',
    'Export episode',
    'Compose a stateless plain-text and Markdown export without changing EchoNote data.',
    {
      episode_id: uuid,
      mode: z.enum(['notes_only', 'organized_note', 'selected_transcript', 'full_transcript']),
      include_user_notes: z.boolean().optional(),
      include_summary: z.boolean().optional(),
      include_key_points: z.boolean().optional(),
      include_worth_reviewing: z.boolean().optional(),
      include_transcript: z.boolean().optional(),
      transcript_segment_ids: z.array(uuid).optional(),
    },
    readOnly,
    ({ episode_id, ...body }) =>
      api.json(`/api/v1/episodes/${encode(episode_id)}/exports`, { method: 'POST', body }),
  )
  tool(
    server,
    'create_conversation',
    'Create episode conversation',
    'Create a durable AI conversation scoped to an episode.',
    { episode_id: uuid, title: z.string().trim().min(1).max(200).optional() },
    write,
    ({ episode_id, title }) =>
      api.json('/api/v1/conversations', { method: 'POST', body: { scope: 'episode', episode_id, title } }),
  )
  tool(
    server,
    'get_conversation',
    'Get conversation',
    'Get a conversation with its durable messages and citations.',
    { conversation_id: uuid },
    readOnly,
    ({ conversation_id }) => api.json(`/api/v1/conversations/${encode(conversation_id)}`),
  )
  tool(
    server,
    'ask_conversation',
    'Ask episode question',
    'Ask a question in an EchoNote conversation and return the completed answer with citations.',
    {
      conversation_id: uuid,
      content: z.string().trim().min(2).max(2000).describe('Question about the episode'),
      client_message_id: uuid.optional().describe('Stable idempotency key; generated when omitted'),
    },
    externalWrite,
    async ({ conversation_id, content, client_message_id }) => {
      const stream = await api.text(`/api/v1/conversations/${encode(conversation_id)}/messages`, {
        method: 'POST',
        accept: 'text/event-stream',
        body: { content, client_message_id: client_message_id ?? randomUUID() },
      })
      let answer = ''
      let completion: unknown
      const citations: unknown[] = []
      for (const event of parseSSE(stream)) {
        const data = JSON.parse(event.data) as Record<string, unknown>
        if (event.event === 'delta') answer += String(data.text ?? '')
        if (event.event === 'citation') citations.push(data)
        if (event.event === 'done') completion = data
        if (event.event === 'error') throw new Error(`${String(data.code ?? 'AI_STREAM_FAILED')}: ${String(data.message ?? '')}`)
      }
      return { answer, citations, completion }
    },
  )
  tool(
    server,
    'start_transcription',
    'Start transcription',
    'Start a versioned transcription job for an episode.',
    {
      episode_id: uuid,
      profile: z.enum(['economy', 'quality']).default('economy'),
      language_hint: z.string().trim().min(2).max(32).optional(),
      speaker_count: z.int().min(1).max(20).optional(),
    },
    externalWrite,
    ({ episode_id, ...body }) =>
      api.json(`/api/v1/episodes/${encode(episode_id)}/transcriptions`, { method: 'POST', body }),
  )
  tool(
    server,
    'get_transcription',
    'Get transcription',
    'Get the current state and progress of a transcription run.',
    { run_id: uuid },
    readOnly,
    ({ run_id }) => api.json(`/api/v1/transcriptions/${encode(run_id)}`),
  )
  tool(
    server,
    'retry_transcription',
    'Retry transcription',
    'Retry only the failed stage or chunks of a transcription run.',
    { run_id: uuid },
    externalWrite,
    ({ run_id }) => api.json(`/api/v1/transcriptions/${encode(run_id)}/retry`, { method: 'POST' }),
  )
  tool(
    server,
    'cancel_transcription',
    'Cancel transcription',
    'Cancel local and queued provider work for a transcription run.',
    { run_id: uuid },
    externalWrite,
    ({ run_id }) => api.json(`/api/v1/transcriptions/${encode(run_id)}/cancel`, { method: 'POST' }),
  )
  tool(
    server,
    'get_episode_transcript',
    'Get episode transcript',
    'Get the active transcript version and its speakers for an episode.',
    { episode_id: uuid },
    readOnly,
    ({ episode_id }) => api.json(`/api/v1/episodes/${encode(episode_id)}/transcript`),
  )
  tool(
    server,
    'list_transcript_segments',
    'List transcript segments',
    'Read an ordered page of transcript segments.',
    {
      transcript_id: uuid,
      limit: z.int().min(1).max(500).default(100),
      offset: z.int().min(0).default(0),
    },
    readOnly,
    ({ transcript_id, limit, offset }) =>
      api.json(query(`/api/v1/transcripts/${encode(transcript_id)}/segments`, { limit, offset })),
  )
  tool(
    server,
    'update_transcript_speaker',
    'Update transcript speaker',
    'Rename a transcript speaker or update the speaker role.',
    {
      transcript_id: uuid,
      speaker_id: uuid,
      display_name: z.string().trim().min(1).max(200),
      role: z.string().trim().max(100).optional(),
    },
    write,
    ({ transcript_id, speaker_id, display_name, role }) =>
      api.json(`/api/v1/transcripts/${encode(transcript_id)}/speakers/${encode(speaker_id)}`, {
        method: 'PATCH',
        body: { display_name, role },
      }),
  )
  tool(
    server,
    'merge_transcript_speakers',
    'Merge transcript speakers',
    'Merge a source speaker into a target speaker; this has no automatic split operation.',
    { transcript_id: uuid, source_speaker_id: uuid, target_speaker_id: uuid },
    destructive,
    ({ transcript_id, source_speaker_id, target_speaker_id }) =>
      api.json(`/api/v1/transcripts/${encode(transcript_id)}/speakers/merge`, {
        method: 'POST',
        body: { source_speaker_id, target_speaker_id },
      }),
  )

  return server
}

function tool<Shape extends z.ZodRawShape>(
  server: McpServer,
  name: string,
  title: string,
  description: string,
  inputSchema: Shape,
  annotations: Annotations,
  handler: (input: z.infer<z.ZodObject<Shape>>) => unknown | Promise<unknown>,
): void {
  const schema = z.object(inputSchema)
  server.registerTool<typeof schema, typeof schema>(
    name,
    { title, description, inputSchema: schema, annotations },
    async (input): Promise<CallToolResult> => {
      try {
        const value = await handler(input as z.infer<z.ZodObject<Shape>>)
        return { content: [{ type: 'text', text: JSON.stringify(value, null, 2) ?? 'null' }] }
      } catch (error) {
        return {
          isError: true,
          content: [{ type: 'text', text: error instanceof Error ? error.message : String(error) }],
        }
      }
    },
  )
}

function encode(value: string): string {
  return encodeURIComponent(value)
}

function query(path: string, values: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams()
  for (const [name, value] of Object.entries(values)) {
    if (value !== undefined) params.set(name, String(value))
  }
  return `${path}?${params}`
}

async function startStdio(api: EchoNoteClient): Promise<void> {
  await createServer(api).connect(new StdioServerTransport())
}

function startHttp(api: EchoNoteClient): void {
  const port = Number(process.env.ECHONOTE_MCP_PORT ?? 3001)
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('ECHONOTE_MCP_PORT must be a valid port')

  const app = createMcpExpressApp({ host: '127.0.0.1' })
  app.get('/healthz', (_request: Request, response: Response) => response.json({ status: 'ok' }))
  app.post('/mcp', async (request: Request, response: Response) => {
    const server = createServer(api)
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined })
    try {
      await server.connect(transport)
      await transport.handleRequest(request, response, request.body)
    } catch (error) {
      console.error(error)
      if (!response.headersSent) {
        response.status(500).json({ jsonrpc: '2.0', error: { code: -32603, message: 'Internal server error' }, id: null })
      }
    } finally {
      await transport.close()
      await server.close()
    }
  })
  app.all('/mcp', (_request: Request, response: Response) => {
    response.status(405).json({ jsonrpc: '2.0', error: { code: -32000, message: 'Method not allowed' }, id: null })
  })
  app.listen(port, '127.0.0.1', () => console.error(`EchoNote MCP listening on http://127.0.0.1:${port}/mcp`))
}

const api = EchoNoteClient.fromEnv()
if (process.argv.includes('--http')) startHttp(api)
else await startStdio(api)
