import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { createMcpExpressApp } from '@modelcontextprotocol/sdk/server/express.js'
import { requireBearerAuth } from '@modelcontextprotocol/sdk/server/auth/middleware/bearerAuth.js'
import {
  getOAuthProtectedResourceMetadataUrl,
  mcpAuthMetadataRouter,
} from '@modelcontextprotocol/sdk/server/auth/router.js'
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js'
import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js'
import type { Request, Response } from 'express'
import * as z from 'zod/v4'
import { EchoNoteClient, type AuthenticatedIdentity } from './api.js'
import { discoverOAuth, JwtTokenVerifier, loadConfig, transcriptionScope, type ServiceConfig } from './auth.js'

type TranscriptionTask = {
  id: string
  source_url: string
  title?: string
  status: string
  stage: string
  completed_chunks: number
  total_chunks: number
  markdown?: string
  error?: { code: string; message: string }
  created_at: string
  updated_at: string
}

const httpUrl = z.url().refine(value => ['http:', 'https:'].includes(new URL(value).protocol), 'Must be an HTTP or HTTPS URL')
const taskId = z.uuid().describe('Task ID returned by transcribe_url')

function createServer(api: EchoNoteClient): McpServer {
  const server = new McpServer(
    { name: 'echonote', version: '0.2.0' },
    { instructions: 'Submit a URL with transcribe_url, then poll get_transcription until it returns Markdown.' },
  )

  server.registerTool(
    'transcribe_url',
    {
      title: 'Transcribe URL',
      description: 'Queue transcription of a podcast page, feed, or direct audio URL.',
      inputSchema: { url: httpUrl.describe('Podcast, feed, page, or direct audio URL') },
      annotations: { readOnlyHint: false, destructiveHint: false, openWorldHint: true },
    },
    async ({ url }): Promise<CallToolResult> =>
      toolResult(() => api.json<TranscriptionTask>('/api/v1/transcriptions', { method: 'POST', body: { url } })),
  )
  server.registerTool(
    'get_transcription',
    {
      title: 'Get transcription',
      description: 'Get transcription progress; returns the complete Markdown transcript when finished.',
      inputSchema: { task_id: taskId },
      annotations: { readOnlyHint: true, destructiveHint: false, openWorldHint: false },
    },
    async ({ task_id }): Promise<CallToolResult> =>
      toolResult(() => api.json<TranscriptionTask>(`/api/v1/transcriptions/${encodeURIComponent(task_id)}`), true),
  )
  return server
}

async function toolResult(action: () => Promise<TranscriptionTask>, markdown = false): Promise<CallToolResult> {
  try {
    const task = await action()
    if (markdown && task.markdown) return { content: [{ type: 'text', text: task.markdown }] }
    const { id: task_id, ...status } = task
    return {
      isError: task.status === 'failed',
      content: [{ type: 'text', text: JSON.stringify({ task_id, ...status }, null, 2) }],
    }
  } catch (error) {
    return { isError: true, content: [{ type: 'text', text: error instanceof Error ? error.message : String(error) }] }
  }
}

async function main(): Promise<void> {
  const config = loadConfig()
  const discovery = await discoverOAuth(config)
  const verifier = new JwtTokenVerifier(config, discovery.jwksUrl)
  const resourceMetadataUrl = getOAuthProtectedResourceMetadataUrl(config.publicUrl)
  const authenticate = requireBearerAuth({ verifier, requiredScopes: [transcriptionScope], resourceMetadataUrl })
  const mcpPath = config.publicUrl.pathname || '/mcp'
  const app = createMcpExpressApp({ host: config.host, allowedHosts: [config.publicUrl.hostname] })

  app.use(
    mcpAuthMetadataRouter({
      oauthMetadata: discovery.metadata,
      resourceServerUrl: config.publicUrl,
      scopesSupported: [transcriptionScope],
      resourceName: 'EchoNote Transcription',
    }),
  )
  app.get('/healthz', (_request: Request, response: Response) => response.json({ status: 'ok' }))
  app.post(mcpPath, authenticate, (request: Request, response: Response) => handleMcp(config, request, response))
  app.all(mcpPath, authenticate, (_request: Request, response: Response) => {
    response.status(405).json({ jsonrpc: '2.0', error: { code: -32000, message: 'Method not allowed' }, id: null })
  })
  app.listen(config.port, config.host, () => console.error(`EchoNote MCP listening at ${config.publicUrl}`))
}

async function handleMcp(config: ServiceConfig, request: Request, response: Response): Promise<void> {
  const identity = requestIdentity(request)
  const api = new EchoNoteClient({ baseUrl: config.apiUrl, internalToken: config.internalToken, identity })
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
}

function requestIdentity(request: Request): AuthenticatedIdentity {
  const issuer = request.auth?.extra?.issuer
  const subject = request.auth?.extra?.subject
  const email = request.auth?.extra?.email
  if (typeof issuer !== 'string' || typeof subject !== 'string') throw new Error('verified OAuth identity is missing')
  return { issuer, subject, ...(typeof email === 'string' ? { email } : {}) }
}

await main()
