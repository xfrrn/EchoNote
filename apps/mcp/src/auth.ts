import type { OAuthTokenVerifier } from '@modelcontextprotocol/sdk/server/auth/provider.js'
import type { AuthInfo } from '@modelcontextprotocol/sdk/server/auth/types.js'
import { OAuthMetadataSchema, type OAuthMetadata } from '@modelcontextprotocol/sdk/shared/auth.js'
import { createRemoteJWKSet, jwtVerify, type JWTVerifyGetKey } from 'jose'

export const transcriptionScope = 'echonote:transcribe'

export type ServiceConfig = {
  publicUrl: URL
  oauthIssuer: URL
  oauthAudience: string
  apiUrl: URL
  internalToken: string
  host: string
  port: number
}

export type OAuthDiscovery = {
  metadata: OAuthMetadata
  jwksUrl: URL
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): ServiceConfig {
  const publicUrl = requiredUrl(env, 'ECHONOTE_PUBLIC_URL')
  const oauthIssuer = requiredUrl(env, 'ECHONOTE_OAUTH_ISSUER')
  const apiUrl = new URL(env.ECHONOTE_API_URL?.trim() || 'http://127.0.0.1:8080')
  const internalToken = env.ECHONOTE_INTERNAL_TOKEN?.trim() || ''
  const host = env.ECHONOTE_MCP_HOST?.trim() || '127.0.0.1'
  const port = Number(env.ECHONOTE_MCP_PORT?.trim() || '3001')

  if (!secureUrl(publicUrl)) throw new Error('ECHONOTE_PUBLIC_URL must use HTTPS (HTTP is allowed only on loopback)')
  if (publicUrl.search || publicUrl.hash) throw new Error('ECHONOTE_PUBLIC_URL must not contain query or fragment')
  if (oauthIssuer.protocol !== 'https:' || oauthIssuer.search || oauthIssuer.hash) {
    throw new Error('ECHONOTE_OAUTH_ISSUER must be an HTTPS issuer URL without query or fragment')
  }
  if (!['127.0.0.1', '::1', 'localhost'].includes(apiUrl.hostname) || !['http:', 'https:'].includes(apiUrl.protocol)) {
    throw new Error('ECHONOTE_API_URL must use HTTP(S) on loopback')
  }
  if (apiUrl.username || apiUrl.password) throw new Error('ECHONOTE_API_URL must not contain credentials')
  if (apiUrl.search || apiUrl.hash) throw new Error('ECHONOTE_API_URL must not contain query or fragment')
  if (internalToken.length < 32 || /^change_?me/i.test(internalToken)) {
    throw new Error('ECHONOTE_INTERNAL_TOKEN must be a non-placeholder secret of at least 32 characters')
  }
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('ECHONOTE_MCP_PORT must be a valid port')

  return {
    publicUrl,
    oauthIssuer,
    oauthAudience: env.ECHONOTE_OAUTH_AUDIENCE?.trim() || publicUrl.toString(),
    apiUrl,
    internalToken,
    host,
    port,
  }
}

export async function discoverOAuth(config: ServiceConfig, fetcher: typeof fetch = fetch): Promise<OAuthDiscovery> {
  let lastError: unknown
  for (const url of discoveryUrls(config.oauthIssuer)) {
    try {
      const response = await fetcher(url, { headers: { Accept: 'application/json' }, signal: AbortSignal.timeout(10_000) })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const raw = (await response.json()) as Record<string, unknown>
      const metadata = OAuthMetadataSchema.parse(raw)
      if (metadata.issuer !== config.oauthIssuer.toString()) throw new Error('discovery issuer does not match ECHONOTE_OAUTH_ISSUER')
      if (!metadata.response_types_supported.includes('code')) throw new Error('authorization server must support code responses')
      if (!metadata.code_challenge_methods_supported?.includes('S256')) throw new Error('authorization server must support PKCE S256')
      if (typeof raw.jwks_uri !== 'string') throw new Error('authorization server metadata is missing jwks_uri')
      const jwksUrl = new URL(raw.jwks_uri)
      if (jwksUrl.protocol !== 'https:' || jwksUrl.username || jwksUrl.password) {
        throw new Error('jwks_uri must use HTTPS without credentials')
      }
      return { metadata, jwksUrl }
    } catch (error) {
      lastError = error
    }
  }
  throw new Error(`OAuth discovery failed: ${lastError instanceof Error ? lastError.message : lastError}`)
}

export class JwtTokenVerifier implements OAuthTokenVerifier {
  private readonly key: JWTVerifyGetKey

  constructor(
    private readonly config: ServiceConfig,
    jwksUrl: URL,
    key: JWTVerifyGetKey = createRemoteJWKSet(jwksUrl),
  ) {
    this.key = key
  }

  async verifyAccessToken(token: string): Promise<AuthInfo> {
    const { payload } = await jwtVerify(token, this.key, {
      issuer: this.config.oauthIssuer.toString(),
      audience: this.config.oauthAudience,
      clockTolerance: 5,
    })
    if (!payload.sub || !payload.iss) throw new Error('access token must contain iss and sub claims')
    const scopes = tokenScopes(payload)
    const clientId = stringClaim(payload.azp) ?? stringClaim(payload.client_id) ?? 'oauth-client'
    return {
      token,
      clientId,
      scopes,
      expiresAt: payload.exp,
      resource: this.config.publicUrl,
      extra: {
        issuer: payload.iss,
        subject: payload.sub,
        ...(stringClaim(payload.email) ? { email: payload.email } : {}),
      },
    }
  }
}

function requiredUrl(env: NodeJS.ProcessEnv, key: string): URL {
  const value = env[key]?.trim()
  if (!value) throw new Error(`${key} is required`)
  const url = new URL(value)
  if (url.username || url.password) throw new Error(`${key} must not contain credentials`)
  return url
}

function secureUrl(url: URL): boolean {
  return url.protocol === 'https:' || (url.protocol === 'http:' && ['127.0.0.1', '::1', 'localhost'].includes(url.hostname))
}

function discoveryUrls(issuer: URL): URL[] {
  const path = issuer.pathname.replace(/\/$/, '')
  return [
    new URL(`${path}/.well-known/openid-configuration`, issuer.origin),
    new URL(`/.well-known/oauth-authorization-server${path}`, issuer.origin),
  ]
}

function tokenScopes(payload: Record<string, unknown>): string[] {
  if (typeof payload.scope === 'string') return payload.scope.split(/\s+/).filter(Boolean)
  if (Array.isArray(payload.scp)) return payload.scp.filter((scope): scope is string => typeof scope === 'string')
  if (typeof payload.scp === 'string') return payload.scp.split(/\s+/).filter(Boolean)
  return []
}

function stringClaim(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}
