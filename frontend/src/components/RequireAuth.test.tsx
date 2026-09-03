import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AuthProvider } from '../auth/AuthContext'
import { RequireAuth } from './RequireAuth'

function renderWith(initial: string, user: unknown) {
  localStorage.setItem('sp_token', user ? 'tok' : '')
  if (user) {
    localStorage.setItem('sp_user', JSON.stringify(user))
  }
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={[initial]}>
        <Routes>
          <Route path="/login" element={<div>login page</div>} />
          <Route element={<RequireAuth />}>
            <Route path="*" element={<div>protected content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AuthProvider>
  )
}

describe('RequireAuth', () => {
  it('renders children when logged in', () => {
    renderWith('/', { id: '1', email: 'a@b.com', display_name: 'A' })
    expect(screen.getByText('protected content')).toBeInTheDocument()
  })

  it('redirects to /login when anonymous', () => {
    renderWith('/', null)
    expect(screen.getByText('login page')).toBeInTheDocument()
    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
  })
})
