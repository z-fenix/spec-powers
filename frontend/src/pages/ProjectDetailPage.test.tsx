import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ProjectDetailPage } from './ProjectDetailPage'
import { apiFetch, ApiError } from '../api/client'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

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
})

describe('ProjectDetailPage', () => {
  it('renders project info, resources and context', async () => {
    mockInitialLoad()
    renderPage()

    expect(await screen.findByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('octo/hello')).toBeInTheDocument()
    expect(screen.getByTestId('context-input')).toHaveValue('team notes')
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
})
