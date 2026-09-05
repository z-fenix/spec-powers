import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { listNotifications } from '../api/notifications'

const POLL_MS = 30_000

export function NotificationBell() {
  const [unread, setUnread] = useState(0)
  const navigate = useNavigate()

  useEffect(() => {
    let cancelled = false
    const refresh = () => {
      listNotifications()
        .then((res) => {
          if (!cancelled) setUnread(res.unread)
        })
        .catch(() => {
          // best effort; the badge just stays stale
        })
    }
    refresh()
    const timer = setInterval(refresh, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [])

  return (
    <button
      data-testid="notification-bell"
      className="notification-bell nav-item"
      aria-label="通知中心"
      onClick={() => navigate('/notifications')}
    >
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
        <path
          d="M2.75 9V4A1.25 1.25 0 0 1 4 2.75h8A1.25 1.25 0 0 1 13.25 4v5m-10.5 0v3A1.25 1.25 0 0 0 4 13.25h8A1.25 1.25 0 0 0 13.25 12V9m-10.5 0h3.1c.3 0 .55.2.63.48a2.06 2.06 0 0 0 4.04 0c.08-.28.34-.48.63-.48h2.1"
          stroke="currentColor"
          strokeWidth="1.3"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      收件箱
      {unread > 0 && <span data-testid="notification-count">{unread}</span>}
    </button>
  )
}
