import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { BoardPage } from './BoardPage'
import * as api from '../api/issues'
import type { Issue } from '../api/issues'

vi.mock('../api/issues', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/issues')>()
  return {
    ...actual,
    listIssues: vi.fn(),
    createIssue: vi.fn(),
    transitionIssue: vi.fn(),
    updateIssue: vi.fn(),
  }
})

vi.mock('../api/workflow', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workflow')>()
  return {
    ...actual,
    getChangeByIssue: vi.fn(),
    listArtifacts: vi.fn(),
  }
})

vi.mock('../api/runs', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/runs')>()
  return {
    ...actual,
    getRun: vi.fn(),
  }
})

vi.mock('../api/statuses', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/statuses')>()
  return {
    ...actual,
    listStatuses: vi.fn(),
    upsertStatus: vi.fn(),
    deleteStatus: vi.fn(),
  }
})

import { getChangeByIssue, listArtifacts } from '../api/workflow'
import type { Change, Artifact } from '../api/workflow'
import { getRun } from '../api/runs'
import type { Run, RunLog } from '../api/runs'
import { listStatuses, upsertStatus, deleteStatus } from '../api/statuses'
import type { WorkspaceStatus } from '../api/statuses'
import { DEFAULT_DIRECTORY } from '../lib/status'

const mocked = vi.mocked(api)

function makeChange(overrides: Partial<Change> = {}): Change {
  return {
    id: 'c1',
    project_id: 'p1',
    issue_id: 'a',
    phase: 'proposal',
    status: 'active',
    created_by: 'u1',
    created_at: '2026-09-05T00:00:00Z',
    updated_at: '2026-09-05T00:00:00Z',
    ...overrides,
  }
}

function makeArtifact(overrides: Partial<Artifact> = {}): Artifact {
  return {
    id: 'art1',
    change_id: 'c1',
    kind: 'proposal',
    version: 1,
    content: '# 提案',
    created_by: 'u1',
    run_id: null,
    created_at: '2026-09-05T00:00:00Z',
    ...overrides,
  }
}

function makeRun(overrides: Partial<Run> = {}): Run {
  return {
    id: 'r1',
    agent_id: 'ag1',
    issue_id: 'a',
    trigger: 'assigned',
    status: 'done',
    error: '',
    created_at: '2026-09-05T00:00:00Z',
    started_at: '2026-09-05T00:00:01Z',
    finished_at: '2026-09-05T00:01:00Z',
    ...overrides,
  }
}

function makeLogs(): RunLog[] {
  return [
    { seq: 1, kind: 'llm_request', content: '规划提案' },
    { seq: 2, kind: 'llm_response', content: '已完成提案撰写' },
  ]
}

function makeIssue(overrides: Partial<Issue>): Issue {
  return {
    id: 'i1',
    project_id: 'p1',
    parent_id: '',
    title: 'an issue',
    description: '',
    status: 'todo',
    priority: 'none',
    assignee_id: '',
    due_date: '',
    labels: [],
    stage: 0,
    position: 0,
    created_by: 'u1',
    ...overrides,
  }
}

function dragProps(id: string) {
  return {
    dataTransfer: {
      getData: () => id,
      setData: vi.fn(),
    } as unknown as DataTransfer,
  }
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/projects/p1/board']}>
      <Routes>
        <Route path="/projects/:id/board" element={<BoardPage />} />
        <Route path="/projects/:id/issues/:issueId" element={<div>issue detail</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listIssues.mockResolvedValue([])
  vi.mocked(listStatuses).mockResolvedValue(DEFAULT_DIRECTORY)
})

function makeStatus(overrides: Partial<WorkspaceStatus>): WorkspaceStatus {
  return { name: 'custom', category: 'todo', position: 0, ...overrides }
}

describe('BoardPage', () => {
  it('renders kanban columns by status and places cards in the right column', async () => {
    mocked.listIssues.mockResolvedValue([
      makeIssue({ id: 'a', title: 'todo card', status: 'todo' }),
      makeIssue({ id: 'b', title: 'doing card', status: 'in_progress' }),
    ])
    renderPage()

    const todoColumn = await screen.findByTestId('column-todo')
    expect(within(todoColumn).getByText('todo card')).toBeInTheDocument()
    const doingColumn = screen.getByTestId('column-in_progress')
    expect(within(doingColumn).getByText('doing card')).toBeInTheDocument()
    expect(within(todoColumn).queryByText('doing card')).not.toBeInTheDocument()
  })

  it('links each card to the issue detail page', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'card a' })])
    renderPage()

    await userEvent.click(await screen.findByText('card a'))

    expect(await screen.findByText('issue detail')).toBeInTheDocument()
  })

  it('groups issues by stage in list view', async () => {
    mocked.listIssues.mockResolvedValue([
      makeIssue({ id: 'a', title: 'stage one', stage: 1 }),
      makeIssue({ id: 'b', title: 'stage two', stage: 2 }),
    ])
    renderPage()

    await userEvent.click(await screen.findByTestId('view-list'))

    const groupOne = screen.getByTestId('stage-group-1')
    expect(within(groupOne).getByText('stage one')).toBeInTheDocument()
    const groupTwo = screen.getByTestId('stage-group-2')
    expect(within(groupTwo).getByText('stage two')).toBeInTheDocument()
  })

  it('reloads with a status filter', async () => {
    renderPage()
    await screen.findByTestId('board')

    await userEvent.selectOptions(screen.getByTestId('filter-status'), 'todo')

    expect(mocked.listIssues).toHaveBeenLastCalledWith('p1', { status: 'todo' })
  })

  it('reloads with a stage filter', async () => {
    renderPage()
    await screen.findByTestId('board')

    await userEvent.type(screen.getByTestId('filter-stage'), '3')

    expect(mocked.listIssues).toHaveBeenLastCalledWith('p1', { stage: 3 })
  })

  it('creates an issue and refreshes the board', async () => {
    mocked.listIssues.mockResolvedValue([])
    mocked.createIssue.mockResolvedValue(makeIssue({ id: 'new', title: 'fresh issue' }))
    renderPage()
    await screen.findByTestId('board')

    await userEvent.click(screen.getByTestId('toggle-create'))
    await userEvent.type(screen.getByTestId('create-title'), 'fresh issue')
    await userEvent.type(screen.getByTestId('create-stage'), '2')
    await userEvent.click(screen.getByTestId('submit-create'))

    expect(mocked.createIssue).toHaveBeenCalledWith('p1', {
      title: 'fresh issue',
      description: '',
      stage: 2,
    })
    await vi.waitFor(() => {
      expect(mocked.listIssues).toHaveBeenCalledTimes(2)
    })
  })

  it('transitions an issue status from its card', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'card a', status: 'todo' })])
    mocked.transitionIssue.mockResolvedValue(makeIssue({ id: 'a', status: 'in_progress' }))
    renderPage()

    await userEvent.selectOptions(
      await screen.findByTestId('status-a'),
      'in_progress',
    )

    expect(mocked.transitionIssue).toHaveBeenCalledWith('p1', 'a', 'in_progress')
  })

  it('flows an issue status when its card is dropped in another column', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'card a', status: 'todo' })])
    mocked.transitionIssue.mockResolvedValue(makeIssue({ id: 'a', status: 'in_progress' }))
    renderPage()

    const card = await screen.findByTestId('card-a')
    fireEvent.dragStart(card, dragProps('a'))
    fireEvent.drop(screen.getByTestId('column-in_progress'), dragProps('a'))

    expect(mocked.transitionIssue).toHaveBeenCalledWith('p1', 'a', 'in_progress')
  })

  it('reorders positions when a card is dropped on another card in the same column', async () => {
    mocked.listIssues.mockResolvedValue([
      makeIssue({ id: 'a', title: 'A', status: 'todo', position: 0 }),
      makeIssue({ id: 'b', title: 'B', status: 'todo', position: 1 }),
      makeIssue({ id: 'c', title: 'C', status: 'todo', position: 2 }),
    ])
    mocked.updateIssue.mockImplementation((_pid, issueId, patch) =>
      Promise.resolve(makeIssue({ id: issueId, position: patch.position ?? 0 })),
    )
    renderPage()

    await screen.findByTestId('card-a')
    fireEvent.dragStart(screen.getByTestId('card-a'), dragProps('a'))
    fireEvent.drop(screen.getByTestId('card-c'), dragProps('a'))

    expect(mocked.updateIssue).toHaveBeenCalledWith('p1', 'b', { position: 0 })
    expect(mocked.updateIssue).toHaveBeenCalledWith('p1', 'c', { position: 1 })
    expect(mocked.updateIssue).toHaveBeenCalledWith('p1', 'a', { position: 2 })

    const todoColumn = screen.getByTestId('column-todo')
    const order = within(todoColumn)
      .getAllByTestId(/^card-/)
      .map((el) => el.dataset.testid)
    expect(order).toEqual(['card-b', 'card-c', 'card-a'])

    await vi.waitFor(() => {
      expect(mocked.listIssues).toHaveBeenCalledTimes(2)
    })
  })

  it('does not touch positions when a card is dropped back on itself', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'A', status: 'todo' })])
    renderPage()

    const card = await screen.findByTestId('card-a')
    fireEvent.dragStart(card, dragProps('a'))
    fireEvent.drop(card, dragProps('a'))

    expect(mocked.updateIssue).not.toHaveBeenCalled()
    expect(mocked.transitionIssue).not.toHaveBeenCalled()
  })

  it('renders kanban columns from the workspace status directory', async () => {
    vi.mocked(listStatuses).mockResolvedValue([
      makeStatus({ name: 'todo', category: 'todo', position: 0 }),
      makeStatus({ name: 'doing', category: 'in_progress', position: 1 }),
      makeStatus({ name: 'qa_review', category: 'in_review', position: 2 }),
      makeStatus({ name: 'shipped', category: 'done', position: 3 }),
    ])
    mocked.listIssues.mockResolvedValue([
      makeIssue({ id: 'a', title: 'qa card', status: 'qa_review' }),
    ])
    renderPage()

    const qaColumn = await screen.findByTestId('column-qa_review')
    expect(within(qaColumn).getByText('qa card')).toBeInTheDocument()
    // columns follow the directory: custom ones appear, dropped built-ins go
    expect(screen.getByTestId('column-doing')).toBeInTheDocument()
    expect(screen.queryByTestId('column-backlog')).not.toBeInTheDocument()
    expect(screen.queryByTestId('column-blocked')).not.toBeInTheDocument()
    // cards offer exactly the directory statuses in their transition select
    expect(within(qaColumn).getByRole('option', { name: 'qa_review' })).toBeInTheDocument()
    expect(within(qaColumn).getByRole('option', { name: '待办' })).toBeInTheDocument()
    expect(within(qaColumn).queryByRole('option', { name: '阻塞' })).not.toBeInTheDocument()
  })

  it('falls back to the built-in columns when the directory request fails', async () => {
    vi.mocked(listStatuses).mockRejectedValue(new Error('boom'))
    renderPage()

    expect(await screen.findByTestId('column-todo')).toBeInTheDocument()
    expect(screen.getByTestId('column-blocked')).toBeInTheDocument()
  })

  it('adds a status through the directory manager and renders its column', async () => {
    const withCustom = [
      ...DEFAULT_DIRECTORY,
      makeStatus({ name: 'qa_review', category: 'in_review', position: 7 }),
    ]
    vi.mocked(upsertStatus).mockResolvedValue(withCustom)
    renderPage()
    await screen.findByTestId('column-todo')

    await userEvent.click(screen.getByTestId('toggle-statuses'))
    await userEvent.type(screen.getByTestId('dir-name'), 'qa_review')
    await userEvent.selectOptions(screen.getByTestId('dir-category'), 'in_review')
    await userEvent.click(screen.getByTestId('dir-submit'))

    expect(vi.mocked(upsertStatus)).toHaveBeenCalledWith('p1', 'qa_review', 'in_review')
    expect(await screen.findByTestId('column-qa_review')).toBeInTheDocument()
  })

  it('deletes a status through the directory manager and drops its column', async () => {
    vi.mocked(deleteStatus).mockResolvedValue(DEFAULT_DIRECTORY.filter((s) => s.name !== 'blocked'))
    renderPage()
    await screen.findByTestId('column-blocked')

    await userEvent.click(screen.getByTestId('toggle-statuses'))
    await userEvent.click(screen.getByTestId('dir-delete-blocked'))

    expect(vi.mocked(deleteStatus)).toHaveBeenCalledWith('p1', 'blocked')
    await vi.waitFor(() => {
      expect(screen.queryByTestId('column-blocked')).not.toBeInTheDocument()
    })
  })

  it('shows the error message when loading fails', async () => {
    mocked.listIssues.mockRejectedValue(new Error('加载失败'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })

  it('lists workflow artifacts on a card with their run logs', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'card a' })])
    vi.mocked(getChangeByIssue).mockResolvedValue(makeChange())
    vi.mocked(listArtifacts).mockResolvedValue([
      makeArtifact({ id: 'art1', run_id: 'r1' }),
    ])
    vi.mocked(getRun).mockResolvedValue({ run: makeRun(), logs: makeLogs() })
    renderPage()

    await userEvent.click(await screen.findByTestId('toggle-artifacts-a'))

    const list = await screen.findByTestId('artifacts-a')
    expect(within(list).getByText('提案')).toBeInTheDocument()
    expect(vi.mocked(getChangeByIssue)).toHaveBeenCalledWith('a')

    await userEvent.click(within(list).getByTestId('logs-art1'))

    const logsList = await screen.findByTestId('logs-art1-list')
    expect(within(logsList).getByText('已完成提案撰写')).toBeInTheDocument()
    expect(vi.mocked(getRun)).toHaveBeenCalledWith('r1')
  })

  it('shows an empty state when the issue has no change', async () => {
    mocked.listIssues.mockResolvedValue([makeIssue({ id: 'a', title: 'card a' })])
    vi.mocked(getChangeByIssue).mockRejectedValue(new Error('not found'))
    renderPage()

    await userEvent.click(await screen.findByTestId('toggle-artifacts-a'))

    expect(await screen.findByTestId('artifacts-empty-a')).toBeInTheDocument()
  })
})
