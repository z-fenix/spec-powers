import { apiFetch } from './client'

export interface SquadMember {
  user_id: string
  display_name: string
  is_agent: boolean
}

export interface Squad {
  id: string
  name: string
  description: string
  leader_id: string
  created_by: string
  members?: SquadMember[]
}

export async function listSquads(): Promise<Squad[]> {
  const res = await apiFetch<{ squads: Squad[] }>('/squads')
  return res.squads ?? []
}

export async function getSquad(id: string): Promise<Squad> {
  const res = await apiFetch<{ squad: Squad }>(`/squads/${id}`)
  return res.squad
}

export async function createSquad(input: { name: string; description?: string; leader_id: string }): Promise<Squad> {
  const res = await apiFetch<{ squad: Squad }>('/squads', { method: 'POST', body: input })
  return res.squad
}

export async function updateSquad(id: string, input: { name?: string; description?: string }): Promise<Squad> {
  const res = await apiFetch<{ squad: Squad }>(`/squads/${id}`, { method: 'PATCH', body: input })
  return res.squad
}

export async function deleteSquad(id: string): Promise<void> {
  await apiFetch<void>(`/squads/${id}`, { method: 'DELETE' })
}

export async function setSquadLeader(id: string, userId: string): Promise<Squad> {
  const res = await apiFetch<{ squad: Squad }>(`/squads/${id}/leader`, { method: 'POST', body: { user_id: userId } })
  return res.squad
}

export async function addSquadMember(id: string, userId: string): Promise<void> {
  await apiFetch<void>(`/squads/${id}/members`, { method: 'POST', body: { user_id: userId } })
}

export async function removeSquadMember(id: string, userId: string): Promise<void> {
  await apiFetch<void>(`/squads/${id}/members/${userId}`, { method: 'DELETE' })
}
