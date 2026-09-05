import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { useTheme } from '../lib/theme'
import { listMembers } from '../api/workspace'
import { NotificationBell } from './NotificationBell'
import { CreateIssueDialog } from './CreateIssueDialog'

function IconSquarePen() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path
        d="M9.5 2.75h-5A1.75 1.75 0 0 0 2.75 4.5v8.75A1.75 1.75 0 0 0 4.5 15h8.75a1.75 1.75 0 0 0 1.75-1.75v-5M10.25 1.75l4 4-6.5 6.5H4.5V9.25l5.75-7.5Z"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

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

function IconWebhook() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path
        d="M5.5 3.5a2 2 0 1 0-1.7 3.1l2 3.4m4.9-6.5a2 2 0 1 1 1.5 3.4L10.5 10M4.5 10.5a2 2 0 1 0 3 1.7h4"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function IconAutopilot() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <circle cx="8" cy="8" r="6" stroke="currentColor" strokeWidth="1.3" />
      <path d="M8 4.5V8l2.5 1.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
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

function IconGear() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <circle cx="8" cy="8" r="2.25" stroke="currentColor" strokeWidth="1.3" />
      <path
        d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
      />
    </svg>
  )
}

function IconSquad() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <circle cx="5.5" cy="5.5" r="2.25" stroke="currentColor" strokeWidth="1.3" />
      <circle cx="11" cy="6.5" r="1.75" stroke="currentColor" strokeWidth="1.3" />
      <path
        d="M1.75 13c.4-2.2 1.9-3.5 3.75-3.5S8.85 10.8 9.25 13M10 9.75c1.6.1 2.9 1.1 3.5 2.75"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
      />
    </svg>
  )
}

function IconAgent() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <rect x="2.75" y="4.75" width="10.5" height="8" rx="1.5" stroke="currentColor" strokeWidth="1.3" />
      <circle cx="5.75" cy="8.25" r="0.9" fill="currentColor" />
      <circle cx="10.25" cy="8.25" r="0.9" fill="currentColor" />
      <path d="M8 2.25V4.5M5.5 11.5h5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  )
}

function IconChevronDown() {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden>
      <path d="M2.5 4.5 6 8l3.5-3.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function IconCheck() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden>
      <path d="m2.5 7.5 3 3 6-7" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function IconLogOut() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden>
      <path
        d="M5.25 12.25h-2A1.25 1.25 0 0 1 2 11V3a1.25 1.25 0 0 1 1.25-1.25h2M9.25 9.5 12.5 7 9.25 4.5M12.25 7h-7"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
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

function useOutsideClose(ref: React.RefObject<HTMLElement | null>, open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [ref, open, onClose])
}

export function Layout() {
  const { user, logout } = useAuth()
  const { theme, resolved, cycle } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [workspaceName, setWorkspaceName] = useState('Spec Powers')
  const menuRef = useRef<HTMLDivElement>(null)

  useOutsideClose(menuRef, menuOpen, () => setMenuOpen(false))

  // close the mobile sidebar and the workspace menu on navigation
  useEffect(() => {
    setSidebarOpen(false)
    setMenuOpen(false)
  }, [location.pathname])

  useEffect(() => {
    let cancelled = false
    listMembers()
      .then((res) => {
        if (!cancelled && res.workspace?.name) setWorkspaceName(res.workspace.name)
      })
      .catch(() => {
        // the fallback name is already in place
      })
    return () => {
      cancelled = true
    }
  }, [])

  const name = user?.display_name || user?.email || ''
  const initial = name ? name[0].toUpperCase() : '?'
  const wsInitial = workspaceName ? workspaceName[0].toUpperCase() : 'S'

  const onLogout = () => {
    logout()
    navigate('/login')
  }

  const navClass = ({ isActive }: { isActive: boolean }) => (isActive ? 'nav-item active' : 'nav-item')

  return (
    <div className={sidebarOpen ? 'shell sidebar-open' : 'shell'}>
      <aside className="sidebar">
        <div className="sidebar-inner">
          <div className="sidebar-header">
            <div className="workspace-switcher" ref={menuRef}>
              <button
                type="button"
                className="sidebar-workspace"
                data-testid="workspace-switcher"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                onClick={() => setMenuOpen((v) => !v)}
              >
                <span className="workspace-avatar">{wsInitial}</span>
                <span className="workspace-name">{workspaceName}</span>
                <span className="workspace-chevron">
                  <IconChevronDown />
                </span>
              </button>
              {menuOpen && (
                <div className="menu" role="menu" data-testid="workspace-menu">
                  <div className="menu-user">
                    <span className="user-avatar user-avatar-lg">{initial}</span>
                    <div className="menu-user-meta">
                      <p className="menu-user-name">{name}</p>
                      {user?.email && <p className="menu-user-email">{user.email}</p>}
                    </div>
                  </div>
                  <div className="menu-separator" />
                  <p className="menu-label">工作空间</p>
                  <button type="button" className="menu-item" role="menuitem" data-testid="menu-current-workspace">
                    <span className="workspace-avatar">{wsInitial}</span>
                    <span className="menu-item-label">{workspaceName}</span>
                    <span className="menu-item-check">
                      <IconCheck />
                    </span>
                  </button>
                  <div className="menu-separator" />
                  <button
                    type="button"
                    className="menu-item menu-item-destructive"
                    role="menuitem"
                    data-testid="menu-logout"
                    onClick={onLogout}
                  >
                    <IconLogOut />
                    <span className="menu-item-label">退出登录</span>
                  </button>
                </div>
              )}
            </div>
            <button
              type="button"
              className="nav-item"
              data-testid="nav-new-task"
              onClick={() => setCreateOpen(true)}
            >
              <IconSquarePen />
              新建任务
            </button>
          </div>
          <nav className="sidebar-nav">
            <NotificationBell />
            <p className="nav-group-label">工作区</p>
            <NavLink to="/" end className={navClass}>
              <IconFolder />
              项目
            </NavLink>
            <NavLink to="/autopilots" className={navClass}>
              <IconAutopilot />
              自动化
            </NavLink>
            <NavLink to="/agents" className={navClass}>
              <IconAgent />
              智能体
            </NavLink>
            <NavLink to="/squads" className={navClass}>
              <IconSquad />
              团队
            </NavLink>
            <p className="nav-group-label">配置</p>
            <NavLink to="/webhooks" className={navClass}>
              <IconWebhook />
              Webhooks
            </NavLink>
            <NavLink to="/settings" className={navClass} data-testid="nav-settings">
              <IconGear />
              设置
            </NavLink>
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
      <CreateIssueDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(projectId, issueId) => navigate(`/projects/${projectId}/issues/${issueId}`)}
      />
    </div>
  )
}
