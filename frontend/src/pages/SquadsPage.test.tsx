import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SquadsPage } from './SquadsPage'
import * as api from '../api/squads'
import type { Squad } from '../api/squads'
import * as agentsApi from '../api/agents'
import type { Agent } from '../api/agents'

vi.mock('../api/squads', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/squads')>()
  return {
    ...actual,
    listSquads: vi.fn(),
    getSquad: vi.fn(),
    createSquad: vi.fn(),
    updateSquad: vi.fn(),
    deleteSquad: vi.fn(),
    setSquadLeader: vi.fn(),
    addSquadMember: vi.fn(),
    removeSquadMember: vi.fn(),
  }
})

vi.mock('../api/agents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/agents')>()
  return { ...actual, listAgents: vi.fn() }
})

const mockedSquads = vi.mocked(api)
const mockedAgents = vi.mocked(agentsApi)

function makeSquad(overrides: Partial<Squad> = {}): Squad {
  return {
    id: 'sq1',
    name: 'Platform',
    description: 'platform work',
    leader_id: 'a1',
    created_by: 'u1',
    members: [
      { user_id: 'a1', display_name: 'KunCoding', is_agent: true },
      { user_id: 'u2', display_name: 'Zhang', is_agent: false },
    ],
    ...overrides,
  }
}

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'a1',
    name: 'KunCoding',
    description: '',
    skills: [],
    runtime: 'server',
    created_by: 'u1',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <SquadsPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedSquads.listSquads.mockReset()
  mockedSquads.getSquad.mockReset()
  mockedAgents.listAgents.mockResolvedValue([makeAgent()])
})

describe('SquadsPage', () => {
  it('shows loading state then squad rows with roster', async () => {
    const squad = makeSquad()
    mockedSquads.listSquads.mockResolvedValueOnce([squad])
    mockedSquads.getSquad.mockResolvedValueOnce(squad)
    renderPage()
    expect(screen.getByText('加载中…')).toBeInTheDocument()
    expect(await screen.findByTestId('squad-row-sq1')).toBeInTheDocument()
    expect(screen.getByText('Platform')).toBeInTheDocument()
    expect(screen.getByTestId('squad-members-sq1')).toHaveTextContent('KunCoding')
    expect(screen.getByTestId('squad-members-sq1')).toHaveTextContent('Zhang')
    expect(screen.getByTestId('squad-leader-sq1')).toHaveTextContent('KunCoding')
  })

  it('shows the empty state when no squads exist', async () => {
    mockedSquads.listSquads.mockResolvedValueOnce([])
    renderPage()
    expect(await screen.findByText('暂无小组。')).toBeInTheDocument()
  })

  it('shows an error when listing fails', async () => {
    mockedSquads.listSquads.mockRejectedValueOnce(new Error('boom'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })

  it('creates a squad with name and leader', async () => {
    const user = userEvent.setup()
    mockedSquads.listSquads.mockResolvedValue([])
    mockedSquads.createSquad.mockResolvedValueOnce(makeSquad())
    renderPage()
    await screen.findByTestId('squad-create')

    await user.type(screen.getByTestId('squad-name-input'), 'Infra')
    await user.selectOptions(screen.getByTestId('squad-leader-select'), 'a1')
    await user.click(screen.getByTestId('squad-create-btn'))

    await waitFor(() => {
      expect(mockedSquads.createSquad).toHaveBeenCalledWith({ name: 'Infra', description: '', leader_id: 'a1' })
    })
  })

  it('removes a roster member', async () => {
    const user = userEvent.setup()
    const squad = makeSquad()
    mockedSquads.listSquads.mockResolvedValue([squad])
    mockedSquads.getSquad.mockResolvedValue(squad)
    mockedSquads.removeSquadMember.mockResolvedValueOnce(undefined)
    renderPage()
    await screen.findByTestId('squad-row-sq1')

    await user.click(screen.getByTestId('squad-remove-u2'))

    await waitFor(() => {
      expect(mockedSquads.removeSquadMember).toHaveBeenCalledWith('sq1', 'u2')
    })
  })

  it('transfers leadership to a roster member', async () => {
    const user = userEvent.setup()
    const squad = makeSquad()
    mockedSquads.listSquads.mockResolvedValue([squad])
    mockedSquads.getSquad.mockResolvedValue(squad)
    mockedSquads.setSquadLeader.mockResolvedValueOnce({ ...squad, leader_id: 'u2' })
    renderPage()
    await screen.findByTestId('squad-row-sq1')

    await user.selectOptions(screen.getByTestId('squad-set-leader-select-sq1'), 'u2')

    await waitFor(() => {
      expect(mockedSquads.setSquadLeader).toHaveBeenCalledWith('sq1', 'u2')
    })
  })

  it('deletes a squad', async () => {
    const user = userEvent.setup()
    const squad = makeSquad()
    mockedSquads.listSquads.mockResolvedValue([squad])
    mockedSquads.getSquad.mockResolvedValue(squad)
    mockedSquads.deleteSquad.mockResolvedValueOnce(undefined)
    renderPage()
    await screen.findByTestId('squad-row-sq1')

    await user.click(screen.getByTestId('squad-delete-sq1'))

    await waitFor(() => {
      expect(mockedSquads.deleteSquad).toHaveBeenCalledWith('sq1')
    })
  })
})
