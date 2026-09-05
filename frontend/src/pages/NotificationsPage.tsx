import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  type Notification,
} from '../api/notifications'

const KIND_LABELS: Record<string, string> = {
  comment: '评论',
  run_finished: '运行完成',
  phase_advanced: '阶段推进',
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function NotificationsPage() {
  const [tab, setTab] = useState<'all' | 'unread'>('all')
  const [notifications, setNotifications] = useState<Notification[] | null>(null)
  const [unread, setUnread] = useState(0)
  const navigate = useNavigate()

  const refresh = useCallback(() => {
    listNotifications(tab === 'unread')
      .then((res) => {
        setNotifications(res.notifications)
        setUnread(res.unread)
      })
      .catch(() => {
        setNotifications([])
      })
  }, [tab])

  useEffect(() => {
    setNotifications(null)
    refresh()
  }, [refresh])

  const onMarkAllRead = async () => {
    await markAllNotificationsRead()
    refresh()
  }

  const onMarkRead = async (n: Notification) => {
    await markNotificationRead(n.id)
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
        收件箱{' '}
        <span data-testid="unread-count" className="badge">
          {unread}
        </span>
      </h2>
      <div className="inbox-tabs" role="tablist">
        <button
          role="tab"
          aria-selected={tab === 'all'}
          data-testid="tab-all"
          onClick={() => setTab('all')}
        >
          全部
        </button>
        <button
          role="tab"
          aria-selected={tab === 'unread'}
          data-testid="tab-unread"
          onClick={() => setTab('unread')}
        >
          未读
        </button>
      </div>
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
              <span className="notification-title">{n.title}</span>{' '}
              <time className="notification-time">{formatTime(n.created_at)}</time>
              {n.body && <p className="notification-body">{n.body}</p>}
              {!n.read && (
                <button
                  data-testid={`read-${n.id}`}
                  onClick={(e) => {
                    e.stopPropagation()
                    onMarkRead(n)
                  }}
                >
                  已读
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
