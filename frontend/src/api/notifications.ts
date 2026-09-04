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

export async function listNotifications(unreadOnly = false): Promise<{ notifications: Notification[]; unread: number }> {
  const path = unreadOnly ? '/notifications?unread=true' : '/notifications'
  return apiFetch<{ notifications: Notification[]; unread: number }>(path)
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
