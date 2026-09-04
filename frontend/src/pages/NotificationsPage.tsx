import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { listNotifications, markAllNotificationsRead, markNotificationRead, type Notification } from '../api/notifications'

const KIND_LABELS: Record<string, string> = {
  comment: '评论',
  run_finished: '运行完成',
  phase_advanced: '阶段推进',
}

export function NotificationsPage() {
  const [notifications, setNotifications] = useState<Notification[] | null>(null)
  const [unread, setUnread] = useState(0)
  const navigate = useNavigate()

  const refresh = useCallback(() => {
    listNotifications()
      .then((res) => {
        setNotifications(res.notifications)
        setUnread(res.unread)
      })
      .catch(() => {
        setNotifications([])
      })
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const onMarkAllRead = async () => {
    await markAllNotificationsRead()
    refresh()
  }

  const onOpen = async (n: Notification) => {
    if (!n.read) {
      await markNotificationRead(n.id)
    }
    if (n.project_id && n.issue_id) {
      navigate(`/projects/${n.project_id}/issues/${n.issue_id}`)
    } else {
      refresh()
    }
  }

  if (notifications === null) return <p>加载中…</p>

  return (
    <section data-testid="notifications-page">
      <h2>
        通知中心{' '}
        <span data-testid="unread-count" className="badge">
          {unread}
        </span>
      </h2>
      <button data-testid="mark-all-read" onClick={onMarkAllRead}>
        全部已读
      </button>
      {notifications.length === 0 ? (
        <p data-testid="notifications-empty">暂无通知</p>
      ) : (
        <ul className="notification-list">
          {notifications.map((n) => (
            <li
              key={n.id}
              data-testid={`notification-${n.id}`}
              data-read={String(n.read)}
              onClick={() => onOpen(n)}
            >
              <span className="notification-kind">{KIND_LABELS[n.kind] ?? n.kind}</span>{' '}
              <span className="notification-title">{n.title}</span>
              {n.body && <p className="notification-body">{n.body}</p>}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
