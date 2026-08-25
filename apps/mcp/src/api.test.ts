import assert from 'node:assert/strict'
import test from 'node:test'
import { EchoNoteApiError, EchoNoteClient, parseSSE } from './api.js'

test('forwards JSON requests without browser authentication state', async () => {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetcher: typeof fetch = async (input, init) => {
    calls.push({ url: String(input), init })
    return new Response('{"id":"import-1"}', { status: 202, headers: { 'Content-Type': 'application/json' } })
  }
  const client = new EchoNoteClient({ baseUrl: 'http://127.0.0.1:8080' }, fetcher)

  assert.deepEqual(await client.json('/api/v1/imports', { method: 'POST', body: { url: 'https://example.com/feed' } }), { id: 'import-1' })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, 'http://127.0.0.1:8080/api/v1/imports')
  assert.equal(calls[0].init?.body, '{"url":"https://example.com/feed"}')
  assert.equal(new Headers(calls[0].init?.headers).get('cookie'), null)
  assert.equal(new Headers(calls[0].init?.headers).get('origin'), null)
})

test('preserves API error details', async () => {
  const client = new EchoNoteClient({ baseUrl: 'http://127.0.0.1:8080' }, async () =>
    new Response('{"code":"EPISODE_NOT_FOUND","message":"episode was not found"}', {
      status: 404,
      headers: { 'X-Request-ID': 'request-1' },
    }),
  )
  await assert.rejects(client.json('/api/v1/episodes/missing'), (error: unknown) =>
    error instanceof EchoNoteApiError && error.status === 404 && error.requestId === 'request-1',
  )
})

test('parses CRLF, comments, and multiline SSE data', () => {
  assert.deepEqual(parseSSE(': connected\r\n\r\nevent: delta\r\ndata: {"text":\r\ndata: "hello"}\r\n\r\n'), [
    { event: 'delta', data: '{"text":\n"hello"}' },
  ])
})
