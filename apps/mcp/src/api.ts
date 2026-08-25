export type AuthenticatedIdentity = {
  issuer: string
  subject: string
  email?: string
}

export type EchoNoteClientConfig = {
  baseUrl: URL
  internalToken: string
  identity: AuthenticatedIdentity
}

type RequestOptions = {
  method?: string
  body?: unknown
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
  constructor(
    private readonly config: EchoNoteClientConfig,
    private readonly fetcher: typeof fetch = fetch,
  ) {}

  async json<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers({
      Accept: 'application/json',
      Authorization: `Bearer ${this.config.internalToken}`,
      'X-EchoNote-Auth-Issuer': this.config.identity.issuer,
      'X-EchoNote-Auth-Subject': this.config.identity.subject,
    })
    if (this.config.identity.email) headers.set('X-EchoNote-Auth-Email', this.config.identity.email)
    if (options.body !== undefined) headers.set('Content-Type', 'application/json')

    let response: Response
    try {
      response = await this.fetcher(new URL(path, this.config.baseUrl), {
        method: options.method ?? 'GET',
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: AbortSignal.timeout(30_000),
      })
    } catch (error) {
      throw new Error(
        `Cannot reach EchoNote API at ${this.config.baseUrl.origin}: ${error instanceof Error ? error.message : error}`,
      )
    }

    if (!response.ok) throw await apiError(response)
    return response.json() as Promise<T>
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
