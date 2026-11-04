import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'

import { problem } from '../test/fixtures'
import { server } from '../test/server'
import { ApiError, isApiError, onSessionExpired, request, setCsrfToken, withQuery } from './client'

describe('request', () => {
  it('sends the CSRF token on writes but not on reads', async () => {
    const seen: Record<string, string | null> = {}
    server.use(
      http.get('/api/thing', ({ request: req }) => {
        seen.get = req.headers.get('X-CSRF-Token')
        return HttpResponse.json({ ok: true })
      }),
      http.post('/api/thing', ({ request: req }) => {
        seen.post = req.headers.get('X-CSRF-Token')
        return new HttpResponse(null, { status: 204 })
      }),
    )
    setCsrfToken('abc')

    await request('/api/thing')
    await request('/api/thing', { method: 'POST' })

    expect(seen).toEqual({ get: null, post: 'abc' })
  })

  it('turns problem+json into an ApiError with code, field errors and Retry-After', async () => {
    server.use(
      http.post('/api/thing', () =>
        HttpResponse.json(
          { status: 429, detail: 'Slow down', code: 'RATE_LIMITED', errors: { email: 'bad' }, requestId: 'r1' },
          { status: 429, headers: { 'Content-Type': 'application/problem+json', 'Retry-After': '42' } },
        ),
      ),
    )

    const error = await request('/api/thing', { method: 'POST', body: {} }).catch((e: unknown) => e)

    expect(isApiError(error, 'RATE_LIMITED')).toBe(true)
    expect(error).toMatchObject({ status: 429, message: 'Slow down', fieldErrors: { email: 'bad' }, requestId: 'r1', retryAfterSeconds: 42 })
  })

  it('reports a network failure without leaking the browser error', async () => {
    server.use(http.get('/api/thing', () => HttpResponse.error()))

    const error = await request('/api/thing').catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 0, code: 'NETWORK' })
  })

  it('announces an expired session only for UNAUTHENTICATED', async () => {
    const listener = vi.fn()
    const unsubscribe = onSessionExpired(listener)
    server.use(
      http.get('/api/expired', () => problem(401, 'UNAUTHENTICATED', 'Sign in')),
      http.post('/api/login', () => problem(401, 'INVALID_CREDENTIALS', 'Wrong')),
    )

    await request('/api/login', { method: 'POST' }).catch(() => undefined)
    expect(listener).not.toHaveBeenCalled()

    await request('/api/expired').catch(() => undefined)
    expect(listener).toHaveBeenCalledOnce()
    unsubscribe()
  })
})

describe('withQuery', () => {
  it('drops empty values', () => {
    expect(withQuery('/api/disputes', { status: undefined, cursor: '', limit: 20 })).toBe('/api/disputes?limit=20')
    expect(withQuery('/api/disputes', {})).toBe('/api/disputes')
  })
})
