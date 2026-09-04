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
      className="notification-bell"
      aria-label="通知中心"
      onClick={() => navigate('/notifications')}
    >
      通知
      {unread > 0 && <span data-testid="notification-count">{unread}</span>}
    </button>
  )
}
