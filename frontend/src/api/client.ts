import type { Problem } from './types'

const GENERIC_ERROR = 'Something went wrong. Please try again.'

export class ApiError extends Error {
  readonly status: number
  readonly code: string | undefined
  readonly fieldErrors: Record<string, string>
  readonly requestId: string | undefined
  readonly retryAfterSeconds: number | undefined

  constructor(status: number, problem: Partial<Problem>, retryAfterSeconds?: number) {
    super(problem.detail ?? GENERIC_ERROR)
    this.name = 'ApiError'
    this.status = status
    this.code = problem.code
    this.fieldErrors = problem.errors ?? {}
    this.requestId = problem.requestId
    this.retryAfterSeconds = retryAfterSeconds
  }
}

export function isApiError(error: unknown, code?: string): error is ApiError {
  return error instanceof ApiError && (code === undefined || error.code === code)
}

// The CSRF token lives in memory only. It is re-issued by /api/auth/me after a
// reload, so it never needs to touch localStorage where injected scripts could read it.
let csrfToken: string | null = null

export function setCsrfToken(token: string | null) {
  csrfToken = token
}

type Listener = () => void
const sessionExpiredListeners = new Set<Listener>()

export function onSessionExpired(listener: Listener): () => void {
  sessionExpiredListeners.add(listener)
  return () => sessionExpiredListeners.delete(listener)
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH'
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json, application/problem+json')
  if (options.body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  if (method !== 'GET' && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }

  let response: Response
  try {
    response = await fetch(new URL(path, window.location.origin), {
      method,
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: 'same-origin',
      signal: options.signal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ApiError(0, {
      code: 'NETWORK',
      detail: "We couldn't reach the server. Check your connection and try again.",
    })
  }

  if (response.status === 204) {
    return undefined as T
  }

  const isJson = response.headers.get('Content-Type')?.includes('json') ?? false
  const data: unknown = isJson ? await response.json().catch(() => null) : null

  if (!response.ok) {
    const retryAfter = Number(response.headers.get('Retry-After')) || undefined
    const error = new ApiError(response.status, (data as Partial<Problem> | null) ?? {}, retryAfter)
    if (error.status === 401 && error.code === 'UNAUTHENTICATED') {
      sessionExpiredListeners.forEach((listener) => listener())
    }
    throw error
  }

  return data as T
}

export function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const query = search.toString()
  return query ? `${path}?${query}` : path
}
