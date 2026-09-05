import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { NotificationsPage } from './NotificationsPage'
import * as api from '../api/notifications'
import type { Notification } from '../api/notifications'

vi.mock('../api/notifications', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/notifications')>()
  return {
    ...actual,
    listNotifications: vi.fn(),
    markNotificationRead: vi.fn(),
    markAllNotificationsRead: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function makeNotification(overrides: Partial<Notification> = {}): Notification {
  return {
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
    ...overrides,
  } as Notification
}

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/notifications']}>
      <Routes>
        <Route path="/notifications" element={<><NotificationsPage /><LocationProbe /></>} />
        <Route path="/projects/:id/issues/:issueId" element={<div>issue page</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listNotifications.mockReset()
})

describe('NotificationsPage', () => {
  it('lists notifications with read state', async () => {
    mocked.listNotifications.mockResolvedValueOnce({
      notifications: [
        makeNotification(),
        makeNotification({ id: 'n2', title: 'Old one', read: true, read_at: '2026-09-04T01:00:00Z' }),
      ],
      unread: 1,
    })

    renderPage()

    expect(await screen.findByTestId('notification-n1')).toBeInTheDocument()
    expect(screen.getByTestId('notification-n2')).toBeInTheDocument()
    expect(screen.getByTestId('notification-n1')).toHaveAttribute('data-read', 'false')
    expect(screen.getByTestId('notification-n2')).toHaveAttribute('data-read', 'true')
  })

  it('marks all notifications read', async () => {
    const user = userEvent.setup()
    mocked.listNotifications
      .mockResolvedValueOnce({ notifications: [makeNotification()], unread: 1 })
      .mockResolvedValueOnce({ notifications: [makeNotification({ read: true })], unread: 0 })
    mocked.markAllNotificationsRead.mockResolvedValueOnce(1)

    renderPage()

    await screen.findByTestId('notification-n1')
    await user.click(screen.getByTestId('mark-all-read'))

    expect(mocked.markAllNotificationsRead).toHaveBeenCalledTimes(1)
    expect(await screen.findByTestId('unread-count')).toHaveTextContent('0')
  })

  it('marks one notification read and opens its issue', async () => {
    const user = userEvent.setup()
    mocked.listNotifications.mockResolvedValueOnce({
      notifications: [makeNotification()],
      unread: 1,
    })
    mocked.markNotificationRead.mockResolvedValueOnce(makeNotification({ read: true }))

    renderPage()

    await screen.findByTestId('notification-n1')
    await user.click(screen.getByTestId('notification-n1'))

    expect(mocked.markNotificationRead).toHaveBeenCalledWith('n1')
    expect(await screen.findByText('issue page')).toBeInTheDocument()
  })

  it('shows an empty state without notifications', async () => {
    mocked.listNotifications.mockResolvedValueOnce({ notifications: [], unread: 0 })

    renderPage()

    expect(await screen.findByTestId('notifications-empty')).toBeInTheDocument()
  })

  it('reloads through the unread filter tab', async () => {
    const user = userEvent.setup()
    mocked.listNotifications.mockResolvedValue({ notifications: [], unread: 0 })

    renderPage()
    await screen.findByTestId('notifications-page')
    expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: undefined })

    await user.click(screen.getByTestId('tab-unread'))
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: true, kind: undefined })
    })

    await user.click(screen.getByTestId('tab-all'))
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: undefined })
    })
  })

  it('filters the inbox by kind', async () => {
    const user = userEvent.setup()
    mocked.listNotifications.mockResolvedValue({ notifications: [], unread: 0 })

    renderPage()
    await screen.findByTestId('notifications-page')
    expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: undefined })

    await user.click(screen.getByTestId('kind-mention'))
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: 'mention' })
    })

    await user.click(screen.getByTestId('kind-due'))
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: 'due' })
    })

    await user.click(screen.getByTestId('kind-all'))
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenLastCalledWith({ unread: false, kind: undefined })
    })
  })

  it('keeps the unread badge and list fresh via polling', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      mocked.listNotifications
        .mockResolvedValueOnce({ notifications: [makeNotification()], unread: 1 })
        .mockResolvedValueOnce({ notifications: [makeNotification({ read: true })], unread: 0 })
        .mockResolvedValue({ notifications: [makeNotification({ read: true })], unread: 0 })

      renderPage()
      expect(await screen.findByTestId('notification-n1')).toHaveAttribute('data-read', 'false')
      expect(screen.getByTestId('unread-count')).toHaveTextContent('1')
      const callsAfterLoad = mocked.listNotifications.mock.calls.length

      await vi.advanceTimersByTimeAsync(30_000)

      expect(mocked.listNotifications.mock.calls.length).toBeGreaterThan(callsAfterLoad)
      expect(await screen.findByTestId('unread-count')).toHaveTextContent('0')
    } finally {
      vi.useRealTimers()
    }
  })

  it('marks one notification read from its read button without navigating', async () => {
    const user = userEvent.setup()
    mocked.listNotifications
      .mockResolvedValueOnce({ notifications: [makeNotification()], unread: 1 })
      .mockResolvedValue({ notifications: [makeNotification({ read: true })], unread: 0 })
    mocked.markNotificationRead.mockResolvedValueOnce(makeNotification({ read: true }))

    renderPage()

    await screen.findByTestId('notification-n1')
    await user.click(screen.getByTestId('read-n1'))

    expect(mocked.markNotificationRead).toHaveBeenCalledWith('n1')
    await vi.waitFor(() => {
      expect(mocked.listNotifications).toHaveBeenCalledTimes(2)
    })
    expect(screen.getByTestId('location')).toHaveTextContent('/notifications')
  })
})
