import { apiFetch } from './client'

export type AgentRuntime = 'server' | 'local'

export interface Agent {
  id: string
  name: string
  description: string
  skills: string[]
  runtime: AgentRuntime | string
  created_by: string
}

export interface AgentInput {
  name: string
  description?: string
  skills?: string[]
}

export interface RegisterResult {
  agent: Agent
  token: string
}

export async function listAgents(): Promise<Agent[]> {
  const res = await apiFetch<{ agents: Agent[] }>('/agents')
  return res.agents ?? []
}

export async function getAgent(agentId: string): Promise<Agent> {
  const res = await apiFetch<{ agent: Agent }>(`/agents/${encodeURIComponent(agentId)}`)
  return res.agent
}

export async function createAgent(input: AgentInput): Promise<Agent> {
  const res = await apiFetch<{ agent: Agent }>('/agents', { method: 'POST', body: input })
  return res.agent
}

export async function registerAgent(input: AgentInput): Promise<RegisterResult> {
  return apiFetch<RegisterResult>('/agents/register', { method: 'POST', body: input })
}

export async function updateAgent(agentId: string, patch: AgentInput): Promise<Agent> {
  const res = await apiFetch<{ agent: Agent }>(`/agents/${encodeURIComponent(agentId)}`, {
    method: 'PATCH',
    body: patch,
  })
  return res.agent
}

export async function deleteAgent(agentId: string): Promise<void> {
  await apiFetch<void>(`/agents/${encodeURIComponent(agentId)}`, { method: 'DELETE' })
}
