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
          d="M8 1.75a4 4 0 0 0-4 4c0 3-1.25 4.5-1.25 4.5h10.5S12 8.75 12 5.75a4 4 0 0 0-4-4ZM6.5 13a1.5 1.5 0 0 0 3 0"
          stroke="currentColor"
          strokeWidth="1.3"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      通知
      {unread > 0 && <span data-testid="notification-count">{unread}</span>}
    </button>
  )
}
