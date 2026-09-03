import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch, ApiError, setToken, clearToken } from './client'

describe('apiFetch', () => {
  beforeEach(() => {
    clearToken()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('attaches bearer token when set', async () => {
    setToken('tok-123')
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), { status: 200 })
    )
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/projects')
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    const headers = init.headers as Record<string, string>
    expect(headers['Authorization']).toBe('Bearer tok-123')
  })

  it('sends JSON body with content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/auth/login', { method: 'POST', body: { email: 'a@b.com' } })
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ email: 'a@b.com' }))
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('parses error envelope into ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'invalid_request', message: 'bad input' } }), { status: 400 })
    ))

    const err = (await apiFetch('/auth/login', { method: 'POST', body: {} }).catch((e) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(400)
    expect(err.code).toBe('invalid_request')
    expect(err.message).toBe('bad input')
  })

  it('clears token and dispatches sp:unauthorized on 401', async () => {
    setToken('stale-token')
    const listener = vi.fn()
    window.addEventListener('sp:unauthorized', listener)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'expired' } }), { status: 401 })
    ))

    const err = (await apiFetch('/me').catch((e) => e)) as ApiError
    expect(err.status).toBe(401)
    expect(localStorage.getItem('sp_token')).toBeNull()
    expect(listener).toHaveBeenCalledTimes(1)
    window.removeEventListener('sp:unauthorized', listener)
  })

  it('returns parsed JSON on success', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ user: { id: '1' } }), { status: 200 })
    ))
    const data = await apiFetch<{ user: { id: string } }>('/me')
    expect(data.user.id).toBe('1')
  })
})
