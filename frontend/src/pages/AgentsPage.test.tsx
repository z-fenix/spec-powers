import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AgentsPage } from './AgentsPage'
import * as api from '../api/agents'
import type { Agent } from '../api/agents'

vi.mock('../api/agents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/agents')>()
  return { ...actual, listAgents: vi.fn() }
})

const mocked = vi.mocked(api)

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'a1',
    name: 'KunCoding',
    description: 'default agent',
    skills: ['code', 'review'],
    runtime: 'server',
    created_by: 'u1',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AgentsPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listAgents.mockReset()
})

describe('AgentsPage', () => {
  it('shows loading state then agent rows', async () => {
    mocked.listAgents.mockResolvedValueOnce([makeAgent()])
    renderPage()
    expect(screen.getByText('加载中…')).toBeInTheDocument()
    expect(await screen.findByTestId('agent-row-a1')).toBeInTheDocument()
    expect(screen.getByText('KunCoding')).toBeInTheDocument()
    expect(screen.getByText('default agent')).toBeInTheDocument()
    expect(screen.getByText('code')).toBeInTheDocument()
  })

  it('shows the empty state when no agents exist', async () => {
    mocked.listAgents.mockResolvedValueOnce([])
    renderPage()
    expect(await screen.findByText('暂无 Agent。')).toBeInTheDocument()
  })

  it('shows an error when listing fails', async () => {
    mocked.listAgents.mockRejectedValueOnce(new Error('boom'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })
})
