import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  listAgents,
  getAgent,
  createAgent,
  registerAgent,
  updateAgent,
  deleteAgent,
  type Agent,
} from './agents'
import { apiFetch } from './client'

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

const agent: Agent = {
  id: 'a1',
  name: 'coder',
  description: '写代码的',
  skills: ['superpowers:test-driven-development'],
  runtime: 'server',
  created_by: 'u1',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

describe('agents api', () => {
  it('lists agents', async () => {
    mockedFetch.mockResolvedValueOnce({ agents: [agent] })
    const res = await listAgents()

    expect(apiFetch).toHaveBeenCalledWith('/agents')
    expect(res).toEqual([agent])
  })

  it('returns an empty list when the payload omits agents', async () => {
    mockedFetch.mockResolvedValueOnce({})
    const res = await listAgents()

    expect(res).toEqual([])
  })

  it('gets one agent', async () => {
    mockedFetch.mockResolvedValueOnce({ agent })
    const res = await getAgent('a1')

    expect(apiFetch).toHaveBeenCalledWith('/agents/a1')
    expect(res).toEqual(agent)
  })

  it('creates a server-runtime agent', async () => {
    mockedFetch.mockResolvedValueOnce({ agent })
    const res = await createAgent({ name: 'coder', description: '写代码的', skills: ['k'] })

    expect(apiFetch).toHaveBeenCalledWith('/agents', {
      method: 'POST',
      body: { name: 'coder', description: '写代码的', skills: ['k'] },
    })
    expect(res).toEqual(agent)
  })

  it('registers a local-runtime agent and returns its token', async () => {
    mockedFetch.mockResolvedValueOnce({ agent, token: 'tok' })
    const res = await registerAgent({ name: 'local bot' })

    expect(apiFetch).toHaveBeenCalledWith('/agents/register', {
      method: 'POST',
      body: { name: 'local bot' },
    })
    expect(res.agent).toEqual(agent)
    expect(res.token).toBe('tok')
  })

  it('patches an agent', async () => {
    mockedFetch.mockResolvedValueOnce({ agent: { ...agent, name: 'renamed' } })
    const res = await updateAgent('a1', { name: 'renamed' })

    expect(apiFetch).toHaveBeenCalledWith('/agents/a1', {
      method: 'PATCH',
      body: { name: 'renamed' },
    })
    expect(res.name).toBe('renamed')
  })

  it('deletes an agent', async () => {
    mockedFetch.mockResolvedValueOnce(undefined)
    await deleteAgent('a1')

    expect(apiFetch).toHaveBeenCalledWith('/agents/a1', { method: 'DELETE' })
  })
})
