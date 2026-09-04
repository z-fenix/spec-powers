import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsPage } from './ProjectsPage'
import { apiFetch } from '../api/client'

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

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <ProjectsPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

describe('ProjectsPage', () => {
  it('loads and renders projects with description and archive state', async () => {
    mockedFetch.mockResolvedValueOnce({ projects: [project] })
    renderPage()

    expect(await screen.findByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('first project')).toBeInTheDocument()
    expect(screen.queryByText('已归档')).not.toBeInTheDocument()
  })

  it('marks archived projects', async () => {
    mockedFetch.mockResolvedValueOnce({ projects: [{ ...project, archived: true }] })
    renderPage()

    expect(await screen.findByText('已归档')).toBeInTheDocument()
  })

  it('creates a project and refreshes the list', async () => {
    const created = { ...project, name: 'Gamma' }
    mockedFetch
      .mockResolvedValueOnce({ projects: [] })
      .mockResolvedValueOnce({ project: created })
      .mockResolvedValueOnce({ projects: [created] })
    renderPage()

    await screen.findByTestId('project-name')
    await userEvent.type(screen.getByTestId('project-name'), 'Gamma')
    await userEvent.type(screen.getByTestId('project-description'), 'new one')
    await userEvent.click(screen.getByTestId('create-project'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects',
      expect.objectContaining({ method: 'POST', body: { name: 'Gamma', description: 'new one' } }),
    )
    expect(await screen.findByText('Gamma')).toBeInTheDocument()
  })

  it('archives a project and refreshes the list', async () => {
    mockedFetch
      .mockResolvedValueOnce({ projects: [project] })
      .mockResolvedValueOnce({ project: { ...project, archived: true } })
      .mockResolvedValueOnce({ projects: [{ ...project, archived: true }] })
    renderPage()

    await userEvent.click(await screen.findByTestId('archive-p1'))

    expect(mockedFetch).toHaveBeenCalledWith(
      '/projects/p1/archive',
      expect.objectContaining({ method: 'POST', body: { archived: true } }),
    )
    expect(await screen.findByText('已归档')).toBeInTheDocument()
  })

  it('shows the error envelope message when loading fails', async () => {
    mockedFetch.mockRejectedValueOnce(new Error('加载失败'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })
})
