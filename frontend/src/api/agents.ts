import { apiFetch } from './client'

export interface Agent {
  id: string
  name: string
  description: string
  skills: string[]
  runtime: string
  created_by: string
}

export async function listAgents(): Promise<Agent[]> {
  const res = await apiFetch<{ agents: Agent[] }>('/agents')
  return res.agents ?? []
}
