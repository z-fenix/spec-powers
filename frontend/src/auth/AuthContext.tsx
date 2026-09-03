import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { apiFetch, setToken, clearToken, getToken } from '../api/client'

export interface User {
  id: string
  email: string
  display_name: string
}

interface AuthState {
  user: User | null
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, displayName: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

const USER_KEY = 'sp_user'

function loadStoredUser(): User | null {
  if (!getToken()) {
    localStorage.removeItem(USER_KEY)
    return null
  }
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? (JSON.parse(raw) as User) : null
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => loadStoredUser())

  useEffect(() => {
    const onUnauthorized = () => {
      localStorage.removeItem(USER_KEY)
      setUser(null)
    }
    window.addEventListener('sp:unauthorized', onUnauthorized)
    return () => window.removeEventListener('sp:unauthorized', onUnauthorized)
  }, [])

  const applySession = useCallback((token: string | null, u: User) => {
    if (token) {
      setToken(token)
    }
    localStorage.setItem(USER_KEY, JSON.stringify(u))
    setUser(u)
  }, [])

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await apiFetch<{ token: string; user: User }>('/auth/login', {
        method: 'POST',
        body: { email, password },
      })
      applySession(res.token, res.user)
    },
    [applySession],
  )

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      const res = await apiFetch<{ user: User }>('/auth/register', {
        method: 'POST',
        body: { email, password, display_name: displayName },
      })
      applySession(null, res.user)
    },
    [applySession],
  )

  const logout = useCallback(() => {
    clearToken()
    localStorage.removeItem(USER_KEY)
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ user, login, register, logout }}>{children}</AuthContext.Provider>
  )
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
