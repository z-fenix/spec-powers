import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  type Notification,
} from '../api/notifications'

const POLL_MS = 30_000

const KIND_LABELS: Record<string, string> = {
  comment: '评论',
  assigned: '指派',
  mention: '提及',
  due: '截止日',
  run_finished: '运行完成',
  phase_advanced: '阶段推进',
  wakeup: '子任务唤醒',
}

const KIND_FILTERS: string[] = ['', 'comment', 'assigned', 'mention', 'due', 'run_finished', 'phase_advanced', 'wakeup']

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function NotificationsPage() {
  const [tab, setTab] = useState<'all' | 'unread'>('all')
  const [kind, setKind] = useState('')
  const [notifications, setNotifications] = useState<Notification[] | null>(null)
  const [unread, setUnread] = useState(0)
  const navigate = useNavigate()

  const refresh = useCallback(() => {
    listNotifications({ unread: tab === 'unread', kind: kind || undefined })
      .then((res) => {
        setNotifications(res.notifications)
        setUnread(res.unread)
      })
      .catch(() => {
        setNotifications([])
      })
  }, [tab, kind])

  useEffect(() => {
    setNotifications(null)
    refresh()
  }, [refresh])

  // Polling refresh: the list and the unread badge track new notifications
  // without user interaction.
  useEffect(() => {
    const timer = setInterval(refresh, POLL_MS)
    return () => clearInterval(timer)
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
    <div className="page" data-testid="notifications-page">
      <div className="page-header">
        <h1 className="page-title">
          收件箱{' '}
          <span data-testid="unread-count" className="badge">
            {unread}
          </span>
        </h1>
      </div>
      <div className="page-body">
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
      <div className="kind-filters" role="group" aria-label="按类型筛选">
        {KIND_FILTERS.map((k) => (
          <button
            key={k || 'all'}
            role="tab"
            aria-selected={kind === k}
            data-testid={k ? `kind-${k}` : 'kind-all'}
            onClick={() => setKind(k)}
          >
            {k ? (KIND_LABELS[k] ?? k) : '全部类型'}
          </button>
        ))}
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
    </div>
    </div>
  )
}
