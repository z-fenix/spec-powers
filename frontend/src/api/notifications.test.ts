import { describe, expect, it, vi, beforeEach } from 'vitest'
import { listNotifications, markNotificationRead, markAllNotificationsRead, type Notification } from './notifications'
import { apiFetch } from './client'

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

const notification: Notification = {
  id: 'n1',
  user_id: 'u1',
  kind: 'comment',
  title: 'New comment on: target',
  body: 'hello',
  issue_id: 'i1',
  project_id: 'p1',
  read: false,
  read_at: null,
  created_at: '2026-09-04T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

describe('notifications api', () => {
  it('lists notifications with the unread count', async () => {
    mockedFetch.mockResolvedValueOnce({ notifications: [notification], unread: 1 })
    const res = await listNotifications()

    expect(apiFetch).toHaveBeenCalledWith('/notifications')
    expect(res.notifications).toEqual([notification])
    expect(res.unread).toBe(1)
  })

  it('lists only unread notifications when asked', async () => {
    mockedFetch.mockResolvedValueOnce({ notifications: [], unread: 0 })
    await listNotifications({ unread: true })

    expect(apiFetch).toHaveBeenCalledWith('/notifications?unread=true')
  })

  it('filters by kind', async () => {
    mockedFetch.mockResolvedValueOnce({ notifications: [], unread: 0 })
    await listNotifications({ kind: 'mention' })

    expect(apiFetch).toHaveBeenCalledWith('/notifications?kind=mention')
  })

  it('combines unread and kind filters', async () => {
    mockedFetch.mockResolvedValueOnce({ notifications: [], unread: 0 })
    await listNotifications({ unread: true, kind: 'due' })

    expect(apiFetch).toHaveBeenCalledWith('/notifications?unread=true&kind=due')
  })

  it('marks one notification read', async () => {
    mockedFetch.mockResolvedValueOnce({ notification: { ...notification, read: true } })
    const res = await markNotificationRead('n1')

    expect(apiFetch).toHaveBeenCalledWith('/notifications/n1/read', { method: 'POST' })
    expect(res.read).toBe(true)
  })

  it('marks all notifications read', async () => {
    mockedFetch.mockResolvedValueOnce({ marked: 3 })
    const res = await markAllNotificationsRead()

    expect(apiFetch).toHaveBeenCalledWith('/notifications/read-all', { method: 'POST' })
    expect(res).toBe(3)
  })
})
