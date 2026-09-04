import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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
  }
})

const mocked = vi.mocked(api)

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
})

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

  it('shows the error message when loading fails', async () => {
    mocked.listIssues.mockRejectedValue(new Error('加载失败'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })
})
