import assert from 'node:assert/strict'
import test from 'node:test'
import { EchoNoteApiError, EchoNoteClient } from './api.js'

const config = {
  baseUrl: new URL('http://127.0.0.1:8080'),
  internalToken: '0123456789abcdef0123456789abcdef',
  identity: { issuer: 'https://login.example.com/', subject: 'user-1', email: 'user@example.com' },
}

test('forwards only the verified identity to the private API', async () => {
  let request: RequestInit | undefined
  const client = new EchoNoteClient(config, async (_input, init) => {
    request = init
    return new Response('{"id":"task-1"}', { status: 202, headers: { 'Content-Type': 'application/json' } })
  })

  assert.deepEqual(
    await client.json('/api/v1/transcriptions', { method: 'POST', body: { url: 'https://example.com/audio.mp3' } }),
    { id: 'task-1' },
  )
  const headers = new Headers(request?.headers)
  assert.equal(headers.get('authorization'), `Bearer ${config.internalToken}`)
  assert.equal(headers.get('x-echonote-auth-issuer'), config.identity.issuer)
  assert.equal(headers.get('x-echonote-auth-subject'), config.identity.subject)
  assert.equal(headers.get('x-echonote-auth-email'), config.identity.email)
  assert.equal(headers.get('cookie'), null)
})

test('preserves private API error details', async () => {
  const client = new EchoNoteClient(config, async () =>
    new Response('{"code":"TRANSCRIPTION_NOT_FOUND","message":"transcription was not found"}', {
      status: 404,
      headers: { 'X-Request-ID': 'request-1' },
    }),
  )
  await assert.rejects(client.json('/api/v1/transcriptions/missing'), (error: unknown) =>
    error instanceof EchoNoteApiError && error.status === 404 && error.requestId === 'request-1',
  )
})
