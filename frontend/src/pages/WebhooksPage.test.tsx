import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { WebhooksPage } from './WebhooksPage'
import * as api from '../api/webhooks'
import type { Webhook } from '../api/webhooks'

vi.mock('../api/webhooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/webhooks')>()
  return {
    ...actual,
    listWebhooks: vi.fn(),
    createWebhook: vi.fn(),
    updateWebhook: vi.fn(),
    deleteWebhook: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function makeWebhook(overrides: Partial<Webhook> = {}): Webhook {
  return {
    id: 'wh1',
    name: 'CI 事件',
    secret: 's3cret-value-123456',
    enabled: true,
    created_at: '2026-09-05T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <WebhooksPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listWebhooks.mockReset()
  mocked.listWebhooks.mockResolvedValue([])
})

describe('WebhooksPage', () => {
  it('shows loading state then webhook rows with endpoint and secret', async () => {
    mocked.listWebhooks.mockResolvedValueOnce([makeWebhook()])
    renderPage()
    expect(screen.getByText('加载中…')).toBeInTheDocument()
    expect(await screen.findByTestId('webhook-row-wh1')).toBeInTheDocument()
    expect(screen.getByText('CI 事件')).toBeInTheDocument()
    expect(screen.getByText('POST /api/v1/hooks/wh1')).toBeInTheDocument()
    expect(screen.getByText('s3cret-value-123456')).toBeInTheDocument()
    expect(screen.getByText('启用')).toBeInTheDocument()
  })

  it('shows the empty state when no webhooks exist', async () => {
    renderPage()
    expect(await screen.findByText('暂无 Webhook。')).toBeInTheDocument()
  })

  it('shows an error when listing fails', async () => {
    mocked.listWebhooks.mockRejectedValueOnce(new Error('boom'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })

  it('creates a webhook through the modal', async () => {
    const user = userEvent.setup()
    mocked.createWebhook.mockResolvedValueOnce(makeWebhook({ id: 'wh2', name: '新事件' }))
    renderPage()
    await user.click(await screen.findByTestId('webhook-create'))
    await user.type(screen.getByTestId('webhook-name'), '新事件')
    mocked.listWebhooks.mockResolvedValueOnce([makeWebhook(), makeWebhook({ id: 'wh2', name: '新事件' })])
    await user.click(screen.getByTestId('webhook-submit'))
    expect(mocked.createWebhook).toHaveBeenCalledWith('新事件')
    await waitFor(() => {
      expect(screen.getByTestId('webhook-row-wh2')).toBeInTheDocument()
    })
  })

  it('toggles a webhook between enabled and disabled', async () => {
    const user = userEvent.setup()
    mocked.listWebhooks.mockResolvedValueOnce([makeWebhook()])
    mocked.updateWebhook.mockResolvedValueOnce(makeWebhook({ enabled: false }))
    renderPage()
    await user.click(await screen.findByTestId('webhook-toggle-wh1'))
    expect(mocked.updateWebhook).toHaveBeenCalledWith('wh1', { enabled: false })
    await waitFor(() => {
      expect(mocked.listWebhooks).toHaveBeenCalledTimes(2)
    })
  })

  it('deletes a webhook', async () => {
    const user = userEvent.setup()
    mocked.listWebhooks.mockResolvedValueOnce([makeWebhook()])
    mocked.deleteWebhook.mockResolvedValueOnce(undefined)
    renderPage()
    await user.click(await screen.findByTestId('webhook-delete-wh1'))
    expect(mocked.deleteWebhook).toHaveBeenCalledWith('wh1')
  })
})
