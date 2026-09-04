import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { NotificationBell } from './NotificationBell'
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
    title: 'New comment',
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

function renderBell() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="*" element={<><NotificationBell /><LocationProbe /></>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listNotifications.mockReset()
})

describe('NotificationBell', () => {
  it('shows the unread count as a badge', async () => {
    mocked.listNotifications.mockResolvedValueOnce({
      notifications: [makeNotification(), makeNotification({ id: 'n2' })],
      unread: 2,
    })

    renderBell()

    expect(await screen.findByTestId('notification-count')).toHaveTextContent('2')
  })

  it('hides the badge when there are no unread notifications', async () => {
    mocked.listNotifications.mockResolvedValueOnce({ notifications: [], unread: 0 })

    renderBell()

    expect(await screen.findByTestId('notification-bell')).toBeInTheDocument()
    expect(screen.queryByTestId('notification-count')).toBeNull()
  })

  it('navigates to the notification center on click', async () => {
    const user = userEvent.setup()
    mocked.listNotifications.mockResolvedValueOnce({ notifications: [], unread: 0 })

    renderBell()

    await screen.findByTestId('notification-bell')
    await user.click(screen.getByTestId('notification-bell'))
    expect(screen.getByTestId('location')).toHaveTextContent('/notifications')
  })
})
