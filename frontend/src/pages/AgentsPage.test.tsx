import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentsPage } from './AgentsPage'
import * as api from '../api/agents'
import type { Agent } from '../api/agents'

vi.mock('../api/agents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/agents')>()
  return {
    ...actual,
    listAgents: vi.fn(),
    createAgent: vi.fn(),
    registerAgent: vi.fn(),
    updateAgent: vi.fn(),
    deleteAgent: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'a1',
    name: 'coder',
    description: '写代码的',
    skills: ['superpowers:test-driven-development'],
    runtime: 'server',
    created_by: 'u1',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listAgents.mockResolvedValue([])
})

describe('AgentsPage', () => {
  it('loads and lists agents with runtime badges and skills', async () => {
    mocked.listAgents.mockResolvedValue([
      makeAgent(),
      makeAgent({ id: 'a2', name: 'local-bot', runtime: 'local', skills: [] }),
    ])
    render(<AgentsPage />)

    await waitFor(() => expect(screen.getByTestId('agents-page')).toBeInTheDocument())
    expect(screen.getByText('coder')).toBeInTheDocument()
    expect(screen.getByTestId('agent-a1')).toHaveTextContent('服务端')
    expect(screen.getByText('local-bot')).toBeInTheDocument()
    expect(screen.getByTestId('agent-a2')).toHaveTextContent('本地')
    expect(screen.getByTestId('agent-skills-a1')).toHaveTextContent(
      'superpowers:test-driven-development',
    )
  })

  it('shows the empty hint when no agents exist', async () => {
    render(<AgentsPage />)

    await waitFor(() =>
      expect(screen.getByText('还没有 Agent，先创建一个。')).toBeInTheDocument(),
    )
  })

  it('creates a server-runtime agent and reloads the list', async () => {
    const user = userEvent.setup()
    mocked.createAgent.mockResolvedValue(makeAgent({ name: 'reviewer', skills: [] }))
    render(<AgentsPage />)

    await screen.findByTestId('agent-name')

    await user.type(screen.getByTestId('agent-name'), 'reviewer')
    await user.type(screen.getByTestId('agent-description'), '审代码')
    await user.type(screen.getByTestId('agent-skills'), 'code-review, spec')
    await user.click(screen.getByTestId('create-agent'))

    await waitFor(() =>
      expect(mocked.createAgent).toHaveBeenCalledWith({
        name: 'reviewer',
        description: '审代码',
        skills: ['code-review', 'spec'],
      }),
    )
    expect(mocked.registerAgent).not.toHaveBeenCalled()
    await waitFor(() => expect(mocked.listAgents).toHaveBeenCalledTimes(2))
  })

  it('registers a local-runtime agent and shows the one-time token', async () => {
    const user = userEvent.setup()
    mocked.registerAgent.mockResolvedValue({ agent: makeAgent({ runtime: 'local' }), token: 'tok-123' })
    render(<AgentsPage />)

    await screen.findByTestId('agent-name')

    await user.selectOptions(screen.getByTestId('agent-runtime'), 'local')
    await user.type(screen.getByTestId('agent-name'), 'local-bot')
    await user.click(screen.getByTestId('create-agent'))

    await waitFor(() =>
      expect(mocked.registerAgent).toHaveBeenCalledWith({
        name: 'local-bot',
        description: '',
        skills: [],
      }),
    )
    expect(mocked.createAgent).not.toHaveBeenCalled()
    expect(screen.getByTestId('register-token')).toHaveTextContent('tok-123')
  })

  it('shows an error when create fails', async () => {
    const user = userEvent.setup()
    mocked.createAgent.mockRejectedValue(new Error('名称已存在'))
    render(<AgentsPage />)

    await screen.findByTestId('agent-name')

    await user.type(screen.getByTestId('agent-name'), 'coder')
    await user.click(screen.getByTestId('create-agent'))

    await waitFor(() => expect(screen.getByTestId('agents-error')).toHaveTextContent('名称已存在'))
  })

  it('edits an agent inline and saves the patch', async () => {
    const user = userEvent.setup()
    mocked.listAgents.mockResolvedValue([makeAgent()])
    mocked.updateAgent.mockResolvedValue(makeAgent({ name: 'renamed', skills: [] }))
    render(<AgentsPage />)

    await waitFor(() => expect(screen.getByTestId('agent-a1')).toBeInTheDocument())
    await user.click(screen.getByTestId('edit-a1'))

    const nameInput = screen.getByTestId('edit-agent-name-a1') as HTMLInputElement
    expect(nameInput.value).toBe('coder')
    await user.clear(nameInput)
    await user.type(nameInput, 'renamed')
    const skillsInput = screen.getByTestId('edit-agent-skills-a1') as HTMLInputElement
    expect(skillsInput.value).toBe('superpowers:test-driven-development')
    await user.clear(skillsInput)
    await user.type(skillsInput, 'a, b')
    await user.click(screen.getByTestId('save-agent-a1'))

    await waitFor(() =>
      expect(mocked.updateAgent).toHaveBeenCalledWith('a1', {
        name: 'renamed',
        description: '写代码的',
        skills: ['a', 'b'],
      }),
    )
    await waitFor(() => expect(mocked.listAgents).toHaveBeenCalledTimes(2))
  })

  it('cancels an inline edit without calling the API', async () => {
    const user = userEvent.setup()
    mocked.listAgents.mockResolvedValue([makeAgent()])
    render(<AgentsPage />)

    await waitFor(() => expect(screen.getByTestId('agent-a1')).toBeInTheDocument())
    await user.click(screen.getByTestId('edit-a1'))
    await user.click(screen.getByTestId('cancel-agent-edit-a1'))

    expect(screen.queryByTestId('edit-agent-name-a1')).not.toBeInTheDocument()
    expect(mocked.updateAgent).not.toHaveBeenCalled()
  })

  it('deletes an agent and reloads', async () => {
    const user = userEvent.setup()
    mocked.listAgents.mockResolvedValue([makeAgent()])
    mocked.deleteAgent.mockResolvedValue(undefined)
    render(<AgentsPage />)

    await waitFor(() => expect(screen.getByTestId('agent-a1')).toBeInTheDocument())
    await user.click(screen.getByTestId('delete-a1'))

    await waitFor(() => expect(mocked.deleteAgent).toHaveBeenCalledWith('a1'))
    await waitFor(() => expect(mocked.listAgents).toHaveBeenCalledTimes(2))
  })
})
