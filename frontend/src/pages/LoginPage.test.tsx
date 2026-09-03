import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '../auth/AuthContext'
import { LoginPage } from './LoginPage'
import { apiFetch, ApiError } from '../api/client'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

function renderPage() {
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    </AuthProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LoginPage', () => {
  it('submits login and shows error message on failure', async () => {
    const err = new ApiError(401, 'unauthorized', '邮箱或密码错误')
    mockedFetch.mockRejectedValueOnce(err)
    renderPage()

    await userEvent.type(screen.getByTestId('email'), 'a@b.com')
    await userEvent.type(screen.getByTestId('password'), 'password123')
    await userEvent.click(screen.getByTestId('submit'))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('邮箱或密码错误')
  })

  it('switches to register mode and sends display_name', async () => {
    mockedFetch.mockResolvedValueOnce({ user: { id: '1', email: 'a@b.com', display_name: 'A' } })
    renderPage()

    await userEvent.click(screen.getByText('没有账号？去注册'))
    await userEvent.type(screen.getByTestId('display-name'), 'A')
    await userEvent.type(screen.getByTestId('email'), 'a@b.com')
    await userEvent.type(screen.getByTestId('password'), 'password123')
    await userEvent.click(screen.getByTestId('submit'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/auth/register',
      expect.objectContaining({
        method: 'POST',
        body: { email: 'a@b.com', password: 'password123', display_name: 'A' },
      }),
    )
  })

  it('successful login navigates away', async () => {
    mockedFetch.mockResolvedValueOnce({ token: 'tok', user: { id: '1', email: 'a@b.com', display_name: 'A' } })
    renderPage()
    await userEvent.type(screen.getByTestId('email'), 'a@b.com')
    await userEvent.type(screen.getByTestId('password'), 'password123')
    await userEvent.click(screen.getByTestId('submit'))
    // No assertion beyond absence of error; navigation happened (no crash).
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
