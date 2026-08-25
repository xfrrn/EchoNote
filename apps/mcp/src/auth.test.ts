import assert from 'node:assert/strict'
import test from 'node:test'
import { createLocalJWKSet, exportJWK, generateKeyPair, SignJWT } from 'jose'
import { JwtTokenVerifier, loadConfig, transcriptionScope } from './auth.js'

const env = {
  ECHONOTE_PUBLIC_URL: 'https://mcp.example.com/mcp',
  ECHONOTE_OAUTH_ISSUER: 'https://login.example.com/',
  ECHONOTE_OAUTH_AUDIENCE: 'https://mcp.example.com/mcp',
  ECHONOTE_API_URL: 'http://127.0.0.1:8080',
  ECHONOTE_INTERNAL_TOKEN: '0123456789abcdef0123456789abcdef',
}

test('rejects public internal API access', () => {
  assert.throws(() => loadConfig({ ...env, ECHONOTE_API_URL: 'https://api.example.com' }), /loopback/)
  assert.throws(() => loadConfig({ ...env, ECHONOTE_INTERNAL_TOKEN: 'CHANGE_ME_0123456789abcdef0123456789' }), /non-placeholder/)
})

test('validates JWT signature, issuer, audience, expiry, and identity', async () => {
  const { privateKey, publicKey } = await generateKeyPair('RS256')
  const jwk = await exportJWK(publicKey)
  const verifier = new JwtTokenVerifier(
    loadConfig(env),
    new URL('https://login.example.com/jwks.json'),
    createLocalJWKSet({ keys: [{ ...jwk, kid: 'test', alg: 'RS256', use: 'sig' }] }),
  )
  const token = await new SignJWT({ scope: transcriptionScope, email: 'user@example.com', azp: 'client-1' })
    .setProtectedHeader({ alg: 'RS256', kid: 'test' })
    .setIssuer(env.ECHONOTE_OAUTH_ISSUER)
    .setSubject('user-1')
    .setAudience(env.ECHONOTE_OAUTH_AUDIENCE)
    .setIssuedAt()
    .setExpirationTime('5m')
    .sign(privateKey)

  const auth = await verifier.verifyAccessToken(token)
  assert.equal(auth.clientId, 'client-1')
  assert.deepEqual(auth.scopes, [transcriptionScope])
  assert.deepEqual(auth.extra, {
    issuer: env.ECHONOTE_OAUTH_ISSUER,
    subject: 'user-1',
    email: 'user@example.com',
  })
  await assert.rejects(verifier.verifyAccessToken(`${token.slice(0, -1)}x`))
})
