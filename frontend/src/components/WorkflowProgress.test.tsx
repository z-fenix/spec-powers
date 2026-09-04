import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowProgress } from './WorkflowProgress'
import * as api from '../api/workflow'
import type { Change, GuardReport } from '../api/workflow'
import { ApiError } from '../api/client'

vi.mock('../api/workflow', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workflow')>()
  return {
    ...actual,
    getChangeByIssue: vi.fn(),
    getGuard: vi.fn(),
    listArtifacts: vi.fn(),
  }
})

const mocked = vi.mocked(api)

const change: Change = {
  id: 'c1',
  project_id: 'p1',
  issue_id: 'i1',
  phase: 'design',
  status: 'active',
  created_by: 'u1',
  created_at: '2026-09-04T00:00:00Z',
  updated_at: '2026-09-04T01:00:00Z',
}

const guard: GuardReport = {
  change_id: 'c1',
  phase: 'design',
  next_phase: 'tasks',
  phase_legal: true,
  handoff_fresh: true,
  verify_passed: false,
  can_advance: true,
  can_archive: false,
  reasons: ['verify report missing'],
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.getChangeByIssue.mockReset()
  mocked.getGuard.mockReset()
  mocked.listArtifacts.mockReset()
})

describe('WorkflowProgress', () => {
  it('renders the four classic phases with the current one active', async () => {
    mocked.getChangeByIssue.mockResolvedValueOnce(change)
    mocked.getGuard.mockResolvedValueOnce(guard)

    render(<WorkflowProgress issueId="i1" />)

    expect(await screen.findByTestId('workflow-progress')).toBeInTheDocument()
    expect(screen.getByTestId('phase-proposal')).toHaveAttribute('data-state', 'done')
    expect(screen.getByTestId('phase-specs')).toHaveAttribute('data-state', 'done')
    expect(screen.getByTestId('phase-design')).toHaveAttribute('data-state', 'current')
    expect(screen.getByTestId('phase-tasks')).toHaveAttribute('data-state', 'upcoming')
  })

  it('renders the guard gate results', async () => {
    mocked.getChangeByIssue.mockResolvedValueOnce(change)
    mocked.getGuard.mockResolvedValueOnce(guard)

    render(<WorkflowProgress issueId="i1" />)

    expect(await screen.findByTestId('guard-gates')).toBeInTheDocument()
    expect(screen.getByTestId('gate-phase_legal')).toHaveAttribute('data-passed', 'true')
    expect(screen.getByTestId('gate-handoff_fresh')).toHaveAttribute('data-passed', 'true')
    expect(screen.getByTestId('gate-verify_passed')).toHaveAttribute('data-passed', 'false')
    expect(screen.getByTestId('gate-can_advance')).toHaveAttribute('data-passed', 'true')
    expect(screen.getByTestId('gate-can_archive')).toHaveAttribute('data-passed', 'false')
  })

  it('lists the guard blocking reasons', async () => {
    mocked.getChangeByIssue.mockResolvedValueOnce(change)
    mocked.getGuard.mockResolvedValueOnce(guard)

    render(<WorkflowProgress issueId="i1" />)

    expect(await screen.findByTestId('guard-reasons')).toHaveTextContent('verify report missing')
  })

  it('renders nothing when the issue has no change', async () => {
    mocked.getChangeByIssue.mockRejectedValueOnce(new ApiError(404, 'not_found', 'change not found'))

    const { container } = render(<WorkflowProgress issueId="i1" />)

    await screen.findByTestId('workflow-empty')
    expect(container.querySelector('[data-testid="workflow-progress"]')).toBeNull()
  })

  it('renders a change in the final phase with tasks done', async () => {
    mocked.getChangeByIssue.mockResolvedValueOnce({ ...change, phase: 'tasks' })
    mocked.getGuard.mockResolvedValueOnce({ ...guard, phase: 'tasks', next_phase: '' })

    render(<WorkflowProgress issueId="i1" />)

    expect(await screen.findByTestId('phase-tasks')).toHaveAttribute('data-state', 'current')
  })
})
