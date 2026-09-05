import { apiFetch } from './client'

export interface Notification {
  id: string
  user_id: string
  kind: string
  title: string
  body: string
  issue_id: string
  project_id: string
  read: boolean
  read_at: string | null
  created_at: string
}

export interface ListOptions {
  unread?: boolean
  kind?: string
}

export async function listNotifications(opts: ListOptions = {}): Promise<{ notifications: Notification[]; unread: number }> {
  const params = new URLSearchParams()
  if (opts.unread) params.set('unread', 'true')
  if (opts.kind) params.set('kind', opts.kind)
  const qs = params.toString()
  return apiFetch<{ notifications: Notification[]; unread: number }>(
    qs ? `/notifications?${qs}` : '/notifications',
  )
}

export async function markNotificationRead(id: string): Promise<Notification> {
  const res = await apiFetch<{ notification: Notification }>(
    `/notifications/${encodeURIComponent(id)}/read`,
    { method: 'POST' },
  )
  return res.notification
}

export async function markAllNotificationsRead(): Promise<number> {
  const res = await apiFetch<{ marked: number }>('/notifications/read-all', { method: 'POST' })
  return res.marked
}
