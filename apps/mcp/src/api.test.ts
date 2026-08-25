import assert from 'node:assert/strict'
import test from 'node:test'
import { EchoNoteClient, parseSSE } from './api.js'

test('logs in once and retries an authenticated request with the session cookie', async () => {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetcher: typeof fetch = async (input, init) => {
    const url = String(input)
    calls.push({ url, init })
    if (url.endsWith('/auth/login')) {
      return new Response('{}', { status: 200, headers: { 'Set-Cookie': 'echonote_session=secret; HttpOnly' } })
    }
    if (calls.length === 1) return new Response('{"code":"AUTH_REQUIRED","message":"authentication required"}', { status: 401 })
    return new Response('{"items":[]}', { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  const client = new EchoNoteClient(
    { baseUrl: 'http://127.0.0.1:8080', username: 'user', password: 'pass' },
    fetcher,
  )

  assert.deepEqual(await client.json('/api/v1/episodes'), { items: [] })
  assert.equal(calls.length, 3)
  assert.equal(new Headers(calls[2].init?.headers).get('cookie'), 'echonote_session=secret')
})

test('parses CRLF, comments, and multiline SSE data', () => {
  assert.deepEqual(parseSSE(': connected\r\n\r\nevent: delta\r\ndata: {"text":\r\ndata: "hello"}\r\n\r\n'), [
    { event: 'delta', data: '{"text":\n"hello"}' },
  ])
})
