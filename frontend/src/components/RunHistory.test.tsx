import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunHistory } from './RunHistory'
import * as api from '../api/runs'
import type { Run, RunLog } from '../api/runs'

vi.mock('../api/runs', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/runs')>()
  return {
    ...actual,
    listRuns: vi.fn(),
    getRun: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function makeRun(overrides: Partial<Run> = {}): Run {
  return {
    id: 'r1',
    agent_id: 'ag1',
    issue_id: 'i1',
    trigger: 'manual',
    status: 'done',
    error: '',
    created_at: '2026-09-05T00:00:00Z',
    started_at: '2026-09-05T00:00:01Z',
    finished_at: '2026-09-05T00:00:31Z',
    ...overrides,
  }
}

function makeLog(overrides: Partial<RunLog> = {}): RunLog {
  return { seq: 1, kind: 'llm_request', content: 'hello world', ...overrides }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('RunHistory', () => {
  it('lists the issue runs with trigger, status and times', async () => {
    mocked.listRuns.mockResolvedValue([
      makeRun(),
      makeRun({ id: 'r2', trigger: 'mention', status: 'failed', error: 'boom' }),
      makeRun({ id: 'r3', trigger: 'assigned', status: 'running', started_at: null, finished_at: null }),
    ])
    render(<RunHistory issueId="i1" />)

    await waitFor(() => expect(screen.getByTestId('run-r1')).toBeInTheDocument())

    expect(mocked.listRuns).toHaveBeenCalledWith({ issue_id: 'i1' })
    expect(screen.getByTestId('run-status-r1')).toHaveTextContent('已完成')
    expect(screen.getByTestId('run-status-r2')).toHaveTextContent('失败')
    expect(screen.getByTestId('run-status-r3')).toHaveTextContent('运行中')
    expect(screen.getAllByText('手动触发').length).toBeGreaterThan(0)
    expect(screen.getAllByText('提及触发').length).toBeGreaterThan(0)
    expect(screen.getAllByText('指派触发').length).toBeGreaterThan(0)
    expect(screen.getByTestId('run-error-r2')).toHaveTextContent('boom')
  })

  it('shows the empty hint when the issue has no runs', async () => {
    mocked.listRuns.mockResolvedValue([])
    render(<RunHistory issueId="i1" />)

    await waitFor(() => expect(screen.getByText('暂无运行记录。')).toBeInTheDocument())
  })

  it('expands a run to load and show its execution logs, then collapses', async () => {
    const user = userEvent.setup()
    mocked.listRuns.mockResolvedValue([makeRun()])
    mocked.getRun.mockResolvedValue({
      run: makeRun(),
      logs: [
        makeLog({ seq: 1, kind: 'llm_request', content: 'first' }),
        makeLog({ seq: 2, kind: 'llm_response', content: 'second' }),
      ],
    })
    render(<RunHistory issueId="i1" />)

    await waitFor(() => expect(screen.getByTestId('run-r1')).toBeInTheDocument())
    expect(mocked.getRun).not.toHaveBeenCalled()

    await user.click(screen.getByTestId('run-toggle-r1'))

    await waitFor(() => expect(mocked.getRun).toHaveBeenCalledWith('r1'))
    await waitFor(() =>
      expect(screen.getByTestId('run-logs-r1')).toHaveTextContent('[llm_request] first'),
    )
    expect(screen.getByTestId('run-logs-r1')).toHaveTextContent('[llm_response] second')

    await user.click(screen.getByTestId('run-toggle-r1'))
    expect(screen.queryByTestId('run-logs-r1')).not.toBeInTheDocument()
  })

  it('shows a hint when an expanded run has no logs', async () => {
    const user = userEvent.setup()
    mocked.listRuns.mockResolvedValue([makeRun()])
    mocked.getRun.mockResolvedValue({ run: makeRun(), logs: [] })
    render(<RunHistory issueId="i1" />)

    await waitFor(() => expect(screen.getByTestId('run-r1')).toBeInTheDocument())
    await user.click(screen.getByTestId('run-toggle-r1'))

    await waitFor(() => expect(screen.getByText('本次运行没有日志。')).toBeInTheDocument())
  })

  it('surfaces a load failure for the run list', async () => {
    mocked.listRuns.mockRejectedValue(new Error('network down'))
    render(<RunHistory issueId="i1" />)

    await waitFor(() =>
      expect(screen.getByTestId('run-history-error')).toHaveTextContent('network down'),
    )
  })

  it('surfaces a log fetch failure inside the expanded run', async () => {
    const user = userEvent.setup()
    mocked.listRuns.mockResolvedValue([makeRun()])
    mocked.getRun.mockRejectedValue(new Error('log fetch failed'))
    render(<RunHistory issueId="i1" />)

    await waitFor(() => expect(screen.getByTestId('run-r1')).toBeInTheDocument())
    await user.click(screen.getByTestId('run-toggle-r1'))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('log fetch failed'),
    )
  })
})
