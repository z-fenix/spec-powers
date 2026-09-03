import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AuthProvider, useAuth } from './AuthContext'
import { apiFetch } from '../api/client'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

function Probe() {
  const { user, login, register, logout } = useAuth()
  const run = (fn: () => Promise<void>) => () => {
    fn().catch(() => {})
  }
  return (
    <div>
      <span data-testid="user">{user ? user.email : 'anon'}</span>
      <button onClick={run(() => login('a@b.com', 'password123'))}>login</button>
      <button onClick={run(() => register('a@b.com', 'password123', 'A'))}>register</button>
      <button onClick={logout}>logout</button>
    </div>
  )
}

function renderProbe(initial = '/') {
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={[initial]}>
        <Routes>
          <Route path="*" element={<Probe />} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>
  )
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('AuthProvider', () => {
  it('login stores user and reports success', async () => {
    mockedFetch.mockResolvedValueOnce({ token: 'tok', user: { id: '1', email: 'a@b.com', display_name: 'A' } })
    renderProbe()
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('a@b.com'))
    expect(mockedFetch).toHaveBeenCalledWith('/auth/login', expect.objectContaining({ method: 'POST' }))
  })

  it('login failure surfaces error and keeps anonymous', async () => {
    mockedFetch.mockRejectedValueOnce(Object.assign(new Error('invalid email or password'), { status: 401 }))
    renderProbe()
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('anon'))
  })

  it('register stores user', async () => {
    mockedFetch.mockResolvedValueOnce({ user: { id: '2', email: 'a@b.com', display_name: 'A' } })
    renderProbe()
    await userEvent.click(screen.getByText('register'))
    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('a@b.com'))
  })

  it('logout clears user', async () => {
    mockedFetch.mockResolvedValueOnce({ token: 'tok', user: { id: '1', email: 'a@b.com', display_name: 'A' } })
    renderProbe()
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('a@b.com'))
    await userEvent.click(screen.getByText('logout'))
    expect(screen.getByTestId('user')).toHaveTextContent('anon')
  })

  it('throws when used outside provider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Probe />)).toThrow()
    spy.mockRestore()
  })
})
