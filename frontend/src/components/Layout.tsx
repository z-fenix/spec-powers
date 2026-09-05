import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { useAuth } from '../auth/AuthContext'
import { useTheme } from '../lib/theme'
import { NotificationBell } from './NotificationBell'

function IconFolder() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path
        d="M1.75 4.5c0-.69.56-1.25 1.25-1.25h3l1.5 1.75h6c.69 0 1.25.56 1.25 1.25v5.25c0 .69-.56 1.25-1.25 1.25H3c-.69 0-1.25-.56-1.25-1.25V4.5Z"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function IconHamburger() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden>
      <path d="M2.5 4.5h13M2.5 9h13M2.5 13.5h13" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

function ThemeIcon({ resolved }: { resolved: 'light' | 'dark' }) {
  if (resolved === 'dark') {
    return (
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
        <path
          d="M13.5 9.5A6 6 0 0 1 6.5 2.5 6 6 0 1 0 13.5 9.5Z"
          stroke="currentColor"
          strokeWidth="1.3"
          strokeLinejoin="round"
        />
      </svg>
    )
  }
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <circle cx="8" cy="8" r="3.25" stroke="currentColor" strokeWidth="1.3" />
      <path
        d="M8 1v1.5M8 13.5V15M1 8h1.5M13.5 8H15M3 3l1 1M12 12l1 1M13 3l-1 1M4 12l-1 1"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
      />
    </svg>
  )
}

const THEME_LABELS: Record<string, string> = {
  light: '浅色',
  dark: '深色',
  system: '跟随系统',
}

export function Layout() {
  const { user, logout } = useAuth()
  const { theme, resolved, cycle } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  // close the mobile sidebar on navigation
  useEffect(() => {
    setSidebarOpen(false)
  }, [location.pathname])

  const name = user?.display_name || user?.email || ''
  const initial = name ? name[0].toUpperCase() : '?'

  return (
    <div className={sidebarOpen ? 'shell sidebar-open' : 'shell'}>
      <aside className="sidebar">
        <div className="sidebar-inner">
          <div className="sidebar-header">
            <div className="sidebar-workspace">
              <span className="workspace-avatar">SP</span>
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                Spec Powers
              </span>
            </div>
          </div>
          <nav className="sidebar-nav">
            <p className="nav-group-label">工作区</p>
            <NavLink to="/" end className={({ isActive }) => (isActive ? 'nav-item active' : 'nav-item')}>
              <IconFolder />
              项目
            </NavLink>
            <NotificationBell />
          </nav>
          <div className="sidebar-footer">
            <button
              type="button"
              className="nav-item"
              data-testid="theme-toggle"
              onClick={cycle}
            >
              <ThemeIcon resolved={resolved} />
              {THEME_LABELS[theme] ?? theme}
            </button>
            <div className="sidebar-user">
              <span className="user-avatar">{initial}</span>
              <span className="sidebar-user-name" data-testid="current-user">
                {name}
              </span>
            </div>
            <button type="button" className="nav-item" onClick={() => { logout(); navigate('/login') }}>
              退出
            </button>
          </div>
        </div>
      </aside>
      {sidebarOpen && (
        <button
          type="button"
          aria-label="关闭菜单"
          className="sidebar-backdrop"
          onClick={() => setSidebarOpen(false)}
        />
      )}
      <div className="main-column">
        <div className="mobile-topbar">
          <button
            type="button"
            className="btn btn-ghost btn-icon"
            aria-label="打开菜单"
            data-testid="sidebar-trigger"
            onClick={() => setSidebarOpen(true)}
          >
            <IconHamburger />
          </button>
          <span className="page-title">Spec Powers</span>
        </div>
        <Outlet />
      </div>
    </div>
  )
}
