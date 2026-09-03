const TOKEN_KEY = 'sp_token'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

const BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api/v1'

export async function apiFetch<T>(path: string, opts: { method?: string; body?: unknown } = {}): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  const res = await fetch(`${BASE}${path}`, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  })

  let payload: unknown = null
  const text = await res.text()
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }

  if (!res.ok) {
    const errBody = (payload as { error?: { code?: string; message?: string } } | null)?.error
    const code = errBody?.code ?? 'unknown'
    const message = errBody?.message ?? `request failed with status ${res.status}`
    if (res.status === 401) {
      clearToken()
      window.dispatchEvent(new CustomEvent('sp:unauthorized'))
    }
    throw new ApiError(res.status, code, message)
  }

  return payload as T
}
