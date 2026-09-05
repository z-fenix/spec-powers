import { Link, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { NotificationBell } from './NotificationBell'

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  return (
    <div className="app-shell">
      <header className="app-header">
        <span className="app-title">Spec Powers</span>
        <nav className="app-nav">
          <Link to="/agents">Agents</Link>
        </nav>
        <div className="app-user">
          <NotificationBell />
          {user && <span data-testid="current-user">{user.display_name || user.email}</span>}
          <button
            onClick={() => {
              logout()
              navigate('/login')
            }}
          >
            退出
          </button>
        </div>
      </header>
      <main className="app-main">
        <Outlet />
      </main>
    </div>
  )
}
