export type EchoNoteClientConfig = {
  baseUrl: string
  origin?: string
  username?: string
  password?: string
}

type RequestOptions = {
  method?: string
  body?: unknown
  accept?: string
}

export class EchoNoteApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestId?: string,
  ) {
    super(`EchoNote API ${status} ${code}: ${message}${requestId ? ` (request ${requestId})` : ''}`)
  }
}

export class EchoNoteClient {
  private readonly baseUrl: URL
  private readonly origin: string
  private readonly username?: string
  private readonly password?: string
  private sessionCookie?: string
  private loginPromise?: Promise<void>

  constructor(config: EchoNoteClientConfig, private readonly fetcher: typeof fetch = fetch) {
    this.baseUrl = new URL(config.baseUrl)
    this.origin = config.origin ?? this.baseUrl.origin
    this.username = config.username
    this.password = config.password
    if (!!this.username !== !!this.password) {
      throw new Error('ECHONOTE_USERNAME and ECHONOTE_PASSWORD must be set together')
    }
  }

  static fromEnv(): EchoNoteClient {
    return new EchoNoteClient({
      baseUrl: process.env.ECHONOTE_API_URL ?? 'http://127.0.0.1:8080',
      origin: process.env.ECHONOTE_API_ORIGIN,
      username: process.env.ECHONOTE_USERNAME,
      password: process.env.ECHONOTE_PASSWORD,
    })
  }

  json(path: string, options: RequestOptions = {}): Promise<unknown> {
    return this.request(path, options, true).then(async response => {
      if (response.status === 204) return { ok: true }
      return response.json()
    })
  }

  text(path: string, options: RequestOptions = {}): Promise<string> {
    return this.request(path, options, true).then(response => response.text())
  }

  private async request(path: string, options: RequestOptions, retryAuth: boolean): Promise<Response> {
    const method = options.method ?? 'GET'
    const headers = new Headers({ Accept: options.accept ?? 'application/json' })
    if (options.body !== undefined) headers.set('Content-Type', 'application/json')
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) headers.set('Origin', this.origin)
    if (this.sessionCookie) headers.set('Cookie', this.sessionCookie)

    let response: Response
    try {
      response = await this.fetcher(new URL(path, this.baseUrl), {
        method,
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
      })
    } catch (error) {
      throw new Error(`Cannot reach EchoNote API at ${this.baseUrl.origin}: ${error instanceof Error ? error.message : error}`)
    }

    if (response.status === 401 && retryAuth && this.username && this.password) {
      await this.ensureLogin()
      return this.request(path, options, false)
    }
    if (!response.ok) throw await apiError(response)
    return response
  }

  private ensureLogin(): Promise<void> {
    if (!this.loginPromise) {
      this.loginPromise = this.login().finally(() => {
        this.loginPromise = undefined
      })
    }
    return this.loginPromise
  }

  private async login(): Promise<void> {
    const response = await this.fetcher(new URL('/api/v1/auth/login', this.baseUrl), {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json', Origin: this.origin },
      body: JSON.stringify({ username: this.username, password: this.password }),
    })
    if (!response.ok) throw await apiError(response)

    const cookie = response.headers.get('set-cookie')?.split(';', 1)[0]
    if (!cookie?.startsWith('echonote_session=')) {
      throw new Error('EchoNote login response did not set echonote_session')
    }
    this.sessionCookie = cookie
    await response.arrayBuffer()
  }
}

async function apiError(response: Response): Promise<EchoNoteApiError> {
  const requestId = response.headers.get('x-request-id') ?? undefined
  const raw = await response.text()
  try {
    const body = JSON.parse(raw) as { code?: string; message?: string }
    return new EchoNoteApiError(response.status, body.code ?? 'HTTP_ERROR', body.message ?? raw, requestId)
  } catch {
    return new EchoNoteApiError(response.status, 'HTTP_ERROR', raw || response.statusText, requestId)
  }
}

export type SSEEvent = { event: string; data: string }

export function parseSSE(payload: string): SSEEvent[] {
  return payload
    .replaceAll('\r\n', '\n')
    .split(/\n\n+/)
    .flatMap(block => {
      let event = 'message'
      const data: string[] = []
      for (const line of block.split('\n')) {
        if (!line || line.startsWith(':')) continue
        const separator = line.indexOf(':')
        const field = separator < 0 ? line : line.slice(0, separator)
        const value = separator < 0 ? '' : line.slice(separator + 1).replace(/^ /, '')
        if (field === 'event') event = value
        if (field === 'data') data.push(value)
      }
      return data.length ? [{ event, data: data.join('\n') }] : []
    })
}
