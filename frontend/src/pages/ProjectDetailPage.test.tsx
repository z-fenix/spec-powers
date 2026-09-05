import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ProjectDetailPage } from './ProjectDetailPage'
import { apiFetch, ApiError } from '../api/client'
import * as runsApi from '../api/runs'
import * as propertiesApi from '../api/properties'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

vi.mock('../api/runs', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/runs')>()
  return { ...actual, getProjectUsage: vi.fn() }
})

vi.mock('../api/properties', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/properties')>()
  return {
    ...actual,
    listPropertyDefinitions: vi.fn(),
    createPropertyDefinition: vi.fn(),
    deletePropertyDefinition: vi.fn(),
  }
})

const mockedFetch = vi.mocked(apiFetch)
const mockedUsage = vi.mocked(runsApi.getProjectUsage)

const project = {
  id: 'p1',
  workspace_id: 'w1',
  name: 'Alpha',
  description: 'first project',
  archived: false,
  created_by: 'u1',
}

const resource = { id: 'r1', type: 'github_repo', label: 'main', pointer: 'octo/hello' }

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/projects/p1']}>
      <Routes>
        <Route path="/projects/:id" element={<ProjectDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

function mockInitialLoad() {
  mockedFetch
    .mockResolvedValueOnce({ project })
    .mockResolvedValueOnce({ resources: [resource] })
    .mockResolvedValueOnce({ context: { content: 'team notes' } })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
  mockedUsage.mockResolvedValue([])
  vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([])
})

describe('ProjectDetailPage', () => {
  it('renders project info, resources and context', async () => {
    mockInitialLoad()
    renderPage()

    expect(await screen.findByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('octo/hello')).toBeInTheDocument()
    expect(screen.getByTestId('context-input')).toHaveValue('team notes')
  })

  it('shows per-issue LLM usage', async () => {
    mockInitialLoad()
    mockedUsage.mockResolvedValue([
      { issue_id: 'i1', title: 'A task', calls: 2, prompt_tokens: 1234, completion_tokens: 567 },
      { issue_id: 'i2', title: 'B task', calls: 1, prompt_tokens: 90, completion_tokens: 40 },
    ])
    renderPage()

    const panel = await screen.findByTestId('project-usage')
    expect(within(panel).getByTestId('usage-row-i1')).toHaveTextContent('A task')
    expect(within(panel).getByTestId('usage-row-i1')).toHaveTextContent('1,234')
    expect(within(panel).getByTestId('usage-row-i2')).toHaveTextContent('B task')
    expect(runsApi.getProjectUsage).toHaveBeenCalledWith('p1')
  })

  it('shows the empty state when no usage was recorded', async () => {
    mockInitialLoad()
    renderPage()

    const panel = await screen.findByTestId('project-usage')
    expect(within(panel).getByText('还没有记录到 LLM 用量。')).toBeInTheDocument()
  })

  it('adds a resource and reloads the list', async () => {
    mockInitialLoad()
    mockedFetch
      .mockResolvedValueOnce({ resource })
      .mockResolvedValueOnce({ resources: [resource] })
    renderPage()

    await userEvent.click(await screen.findByTestId('add-resource'))
    await userEvent.type(await screen.findByTestId('resource-label'), 'lib')
    await userEvent.type(screen.getByTestId('resource-pointer'), 'octo/lib')
    await userEvent.click(screen.getByTestId('confirm-add-resource'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects/p1/resources',
      expect.objectContaining({
        method: 'POST',
        body: { type: 'github_repo', label: 'lib', pointer: 'octo/lib' },
      }),
    )
  })

  it('adds a worktree resource with branch and path', async () => {
    mockInitialLoad()
    const worktree = { id: 'r2', type: 'worktree', label: 'wt', pointer: '/repos/demo', branch: 'feature', path: '/repos/demo-wt' }
    mockedFetch
      .mockResolvedValueOnce({ resource: worktree })
      .mockResolvedValueOnce({ resources: [worktree] })
    renderPage()

    await userEvent.click(await screen.findByTestId('add-resource'))
    await userEvent.selectOptions(screen.getByTestId('resource-type'), 'worktree')
    await userEvent.type(await screen.findByTestId('resource-label'), 'wt')
    await userEvent.type(screen.getByTestId('resource-pointer'), '/repos/demo')
    await userEvent.type(screen.getByTestId('resource-branch'), 'feature')
    await userEvent.type(screen.getByTestId('resource-path'), '/repos/demo-wt')
    await userEvent.click(screen.getByTestId('confirm-add-resource'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects/p1/resources',
      expect.objectContaining({
        method: 'POST',
        body: { type: 'worktree', label: 'wt', pointer: '/repos/demo', branch: 'feature', path: '/repos/demo-wt' },
      }),
    )
    expect(await screen.findByText('Worktree')).toBeInTheDocument()
    expect(screen.getByTestId('worktree-branch-r2')).toHaveTextContent('feature → /repos/demo-wt')
  })

  it('hides worktree fields for non-worktree resource types', async () => {
    mockInitialLoad()
    renderPage()

    await userEvent.click(await screen.findByTestId('add-resource'))
    await userEvent.selectOptions(screen.getByTestId('resource-type'), 'github_repo')

    expect(screen.queryByTestId('resource-branch')).not.toBeInTheDocument()
    expect(screen.queryByTestId('resource-path')).not.toBeInTheDocument()
  })

  it('removes a resource', async () => {
    mockInitialLoad()
    mockedFetch
      .mockResolvedValueOnce(undefined) // DELETE → 204
      .mockResolvedValueOnce({ resources: [] })
    renderPage()

    await userEvent.click(await screen.findByTestId('remove-r1'))

    expect(mockedFetch).toHaveBeenCalledWith('/projects/p1/resources/r1', expect.objectContaining({ method: 'DELETE' }))
    expect(await screen.findByText('还没有绑定资源。')).toBeInTheDocument()
  })

  it('saves the project context', async () => {
    mockInitialLoad()
    mockedFetch.mockResolvedValueOnce({ context: { content: 'updated' } })
    renderPage()

    await userEvent.clear(await screen.findByTestId('context-input'))
    await userEvent.type(screen.getByTestId('context-input'), 'updated')
    await userEvent.click(screen.getByTestId('save-context'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects/p1/context',
      expect.objectContaining({ method: 'PUT', body: { content: 'updated' } }),
    )
  })

  it('archives the project from the detail page', async () => {
    mockInitialLoad()
    mockedFetch
      .mockResolvedValueOnce({ project: { ...project, archived: true } })
      .mockResolvedValueOnce({ project: { ...project, archived: true } })
      .mockResolvedValueOnce({ resources: [resource] })
      .mockResolvedValueOnce({ context: { content: 'team notes' } })
    renderPage()

    await userEvent.click(await screen.findByTestId('toggle-archive'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects/p1/archive',
      expect.objectContaining({ method: 'POST', body: { archived: true } }),
    )
    expect((await screen.findAllByText('已归档')).length).toBeGreaterThan(0)
  })

  it('offers restore for an archived project', async () => {
    mockedFetch
      .mockResolvedValueOnce({ project: { ...project, archived: true } })
      .mockResolvedValueOnce({ resources: [] })
      .mockResolvedValueOnce({ context: { content: '' } })
    renderPage()

    expect(await screen.findByTestId('toggle-archive')).toHaveTextContent('恢复')
    expect(screen.getByText('已归档')).toBeInTheDocument()
  })

  it('shows the api error when adding an invalid resource', async () => {
    mockInitialLoad()
    mockedFetch.mockRejectedValueOnce(new ApiError(400, 'invalid_request', 'invalid github_repo pointer'))
    renderPage()

    await userEvent.click(await screen.findByTestId('add-resource'))
    await userEvent.type(await screen.findByTestId('resource-label'), 'bad')
    await userEvent.type(screen.getByTestId('resource-pointer'), 'nope')
    await userEvent.click(screen.getByTestId('confirm-add-resource'))

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid github_repo pointer')
  })

  it('creates a select property definition', async () => {
    mockInitialLoad()
    vi.mocked(propertiesApi.listPropertyDefinitions)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        {
          id: 'prop1',
          project_id: 'p1',
          name: '模块',
          type: 'select',
          options: ['前端', '后端'],
          position: 0,
        },
      ])
    vi.mocked(propertiesApi.createPropertyDefinition).mockResolvedValue({
      id: 'prop1',
      project_id: 'p1',
      name: '模块',
      type: 'select',
      options: ['前端', '后端'],
      position: 0,
    })
    renderPage()

    await userEvent.type(await screen.findByTestId('property-name'), '模块')
    await userEvent.selectOptions(screen.getByTestId('property-type'), 'select')
    await userEvent.type(screen.getByTestId('property-options'), '前端, 后端')
    await userEvent.click(screen.getByTestId('add-property'))

    expect(propertiesApi.createPropertyDefinition).toHaveBeenCalledWith('p1', {
      name: '模块',
      type: 'select',
      options: ['前端', '后端'],
    })
    expect(await screen.findByTestId('property-def-prop1')).toBeInTheDocument()
    expect(screen.getByText(/前端 \/ 后端/)).toBeInTheDocument()
  })

  it('sends no options for a text property', async () => {
    mockInitialLoad()
    vi.mocked(propertiesApi.createPropertyDefinition).mockResolvedValue({
      id: 'prop2',
      project_id: 'p1',
      name: '备注',
      type: 'text',
      options: [],
      position: 1,
    })
    renderPage()

    await userEvent.type(await screen.findByTestId('property-name'), '备注')
    await userEvent.selectOptions(screen.getByTestId('property-type'), 'text')
    await userEvent.click(screen.getByTestId('add-property'))

    expect(propertiesApi.createPropertyDefinition).toHaveBeenCalledWith('p1', {
      name: '备注',
      type: 'text',
      options: undefined,
    })
  })

  it('deletes a property definition', async () => {
    mockInitialLoad()
    vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([
      {
        id: 'prop1',
        project_id: 'p1',
        name: '模块',
        type: 'select',
        options: ['前端'],
        position: 0,
      },
    ])
    vi.mocked(propertiesApi.deletePropertyDefinition).mockResolvedValue(undefined)
    renderPage()

    await userEvent.click(await screen.findByTestId('delete-property-prop1'))

    expect(propertiesApi.deletePropertyDefinition).toHaveBeenCalledWith('p1', 'prop1')
    await vi.waitFor(() => {
      expect(propertiesApi.listPropertyDefinitions).toHaveBeenCalledTimes(2)
    })
  })
})
