import assert from 'node:assert/strict'
import test from 'node:test'
import { createLocalJWKSet, exportJWK, generateKeyPair, SignJWT } from 'jose'
import { discoverOAuth, JwtTokenVerifier, loadConfig, transcriptionScope } from './auth.js'

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

test('allows an HTTP OAuth issuer only on loopback outside production', async () => {
  const local = {
    ...env,
    APP_ENV: 'development',
    ECHONOTE_PUBLIC_URL: 'http://127.0.0.1:3001/mcp',
    ECHONOTE_OAUTH_ISSUER: 'http://127.0.0.1:8081/realms/echonote',
    ECHONOTE_OAUTH_AUDIENCE: 'http://127.0.0.1:3001/mcp',
  }
  const config = loadConfig(local)
  assert.equal(config.allowInsecureOAuth, true)
  const discovery = await discoverOAuth(config, async () =>
    new Response(JSON.stringify({
      issuer: local.ECHONOTE_OAUTH_ISSUER,
      authorization_endpoint: `${local.ECHONOTE_OAUTH_ISSUER}/protocol/openid-connect/auth`,
      token_endpoint: `${local.ECHONOTE_OAUTH_ISSUER}/protocol/openid-connect/token`,
      jwks_uri: `${local.ECHONOTE_OAUTH_ISSUER}/protocol/openid-connect/certs`,
      response_types_supported: ['code'],
      code_challenge_methods_supported: ['S256'],
    })),
  )
  assert.equal(discovery.jwksUrl.protocol, 'http:')
  assert.throws(() => loadConfig({ ...local, APP_ENV: 'production' }), /must use HTTPS/)
  assert.throws(() => loadConfig({ ...local, ECHONOTE_OAUTH_ISSUER: 'http://192.168.1.2:8081/realms/echonote' }), /must use HTTPS/)
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
  const [header, payload, signature] = token.split('.')
  const tamperedSignature = `${signature.startsWith('A') ? 'B' : 'A'}${signature.slice(1)}`
  await assert.rejects(verifier.verifyAccessToken(`${header}.${payload}.${tamperedSignature}`))
})
