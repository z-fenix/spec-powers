import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AutopilotsPage } from './AutopilotsPage'
import * as api from '../api/autopilots'
import * as agentsApi from '../api/agents'
import { apiFetch } from '../api/client'
import type { Autopilot } from '../api/autopilots'

vi.mock('../api/autopilots', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/autopilots')>()
  return {
    ...actual,
    listAutopilots: vi.fn(),
    createAutopilot: vi.fn(),
    updateAutopilot: vi.fn(),
    deleteAutopilot: vi.fn(),
    triggerAutopilot: vi.fn(),
  }
})

vi.mock('../api/agents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/agents')>()
  return { ...actual, listAgents: vi.fn() }
})

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mocked = vi.mocked(api)
const mockedAgents = vi.mocked(agentsApi)
const mockedFetch = vi.mocked(apiFetch)

function makeAutopilot(overrides: Partial<Autopilot> = {}): Autopilot {
  return {
    id: 'ap1',
    name: '每日报表',
    trigger_type: 'cron',
    cron_spec: '0 9 * * *',
    webhook_id: '',
    action_type: 'create_issue',
    agent_id: '',
    project_id: 'p1',
    issue_id: '',
    issue_title: '日报',
    issue_description: '',
    enabled: true,
    last_fired_at: null,
    next_run_at: '2026-09-06T09:00:00Z',
    created_at: '2026-09-05T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AutopilotsPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listAutopilots.mockReset().mockResolvedValue([])
  mockedAgents.listAgents.mockResolvedValue([{ id: 'a1', name: 'KunCoding', description: '', skills: [], runtime: 'server', created_by: 'u1' }])
  mockedFetch.mockImplementation(async (path: string) => {
    if (path === '/webhooks') return { webhooks: [{ id: 'wh1', name: 'CI', secret: 'x', enabled: true, created_at: '' }] }
    if (path === '/projects') return { projects: [{ id: 'p1', name: 'Spec Powers' }] }
    throw new Error(`unexpected fetch: ${path}`)
  })
})

describe('AutopilotsPage', () => {
  it('shows loading state then autopilot rows with trigger and action badges', async () => {
    mocked.listAutopilots.mockResolvedValueOnce([makeAutopilot()])
    renderPage()
    expect(screen.getByText('加载中…')).toBeInTheDocument()
    expect(await screen.findByTestId('autopilot-row-ap1')).toBeInTheDocument()
    expect(screen.getByText('每日报表')).toBeInTheDocument()
    expect(screen.getByText('定时 · 0 9 * * *')).toBeInTheDocument()
    expect(screen.getByText('创建 Issue')).toBeInTheDocument()
  })

  it('shows the empty state when no autopilots exist', async () => {
    renderPage()
    expect(await screen.findByText('暂无 Autopilot。')).toBeInTheDocument()
  })

  it('shows an error when listing fails', async () => {
    mocked.listAutopilots.mockRejectedValueOnce(new Error('boom'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })

  it('creates a cron autopilot through the modal', async () => {
    const user = userEvent.setup()
    mocked.createAutopilot.mockResolvedValueOnce(makeAutopilot({ id: 'ap2' }))
    renderPage()
    await user.click(await screen.findByTestId('autopilot-create'))
    await user.type(screen.getByTestId('autopilot-name'), '每日报表')
    await user.selectOptions(screen.getByTestId('autopilot-trigger-type'), 'cron')
    expect(screen.getByTestId('autopilot-cron')).toBeInTheDocument()
    await user.type(screen.getByTestId('autopilot-cron'), '0 9 * * *')
    await user.selectOptions(screen.getByTestId('autopilot-project'), 'p1')
    await user.type(screen.getByTestId('autopilot-issue-title'), '日报')
    mocked.listAutopilots.mockResolvedValueOnce([makeAutopilot(), makeAutopilot({ id: 'ap2' })])
    await user.click(screen.getByTestId('autopilot-submit'))
    await waitFor(() => {
      expect(mocked.createAutopilot).toHaveBeenCalledWith(
        expect.objectContaining({
          name: '每日报表',
          trigger_type: 'cron',
          cron_spec: '0 9 * * *',
          project_id: 'p1',
          issue_title: '日报',
        }),
      )
    })
    await waitFor(() => {
      expect(screen.getByTestId('autopilot-row-ap2')).toBeInTheDocument()
    })
  })

  it('shows webhook selector only for webhook trigger', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByTestId('autopilot-create'))
    expect(screen.queryByTestId('autopilot-cron')).not.toBeInTheDocument()
    expect(screen.queryByTestId('autopilot-webhook')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByTestId('autopilot-trigger-type'), 'webhook')
    expect(screen.getByTestId('autopilot-webhook')).toBeInTheDocument()
    await user.selectOptions(screen.getByTestId('autopilot-trigger-type'), 'cron')
    expect(screen.getByTestId('autopilot-cron')).toBeInTheDocument()
  })

  it('shows agent selector for run_agent action', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByTestId('autopilot-create'))
    await user.selectOptions(screen.getByTestId('autopilot-action-type'), 'run_agent')
    expect(screen.getByTestId('autopilot-agent')).toBeInTheDocument()
    expect(screen.getByTestId('autopilot-issue-id')).toBeInTheDocument()
    expect(screen.queryByTestId('autopilot-project')).not.toBeInTheDocument()
  })

  it('manually triggers an autopilot', async () => {
    const user = userEvent.setup()
    mocked.listAutopilots.mockResolvedValueOnce([makeAutopilot()])
    renderPage()
    await user.click(await screen.findByTestId('autopilot-trigger-ap1'))
    expect(mocked.triggerAutopilot).toHaveBeenCalledWith('ap1')
  })

  it('toggles an autopilot enabled state', async () => {
    const user = userEvent.setup()
    mocked.listAutopilots.mockResolvedValueOnce([makeAutopilot()])
    mocked.updateAutopilot.mockResolvedValueOnce(makeAutopilot({ enabled: false }))
    renderPage()
    await user.click(await screen.findByTestId('autopilot-toggle-ap1'))
    expect(mocked.updateAutopilot).toHaveBeenCalledWith('ap1', expect.objectContaining({ enabled: false }))
  })

  it('deletes an autopilot', async () => {
    const user = userEvent.setup()
    mocked.listAutopilots.mockResolvedValueOnce([makeAutopilot()])
    mocked.deleteAutopilot.mockResolvedValueOnce(undefined)
    renderPage()
    await user.click(await screen.findByTestId('autopilot-delete-ap1'))
    expect(mocked.deleteAutopilot).toHaveBeenCalledWith('ap1')
  })
})
