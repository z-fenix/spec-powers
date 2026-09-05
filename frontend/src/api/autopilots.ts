import { apiFetch } from './client'

export interface Autopilot {
  id: string
  name: string
  trigger_type: 'cron' | 'webhook' | 'manual'
  cron_spec: string
  webhook_id: string
  action_type: 'create_issue' | 'run_agent'
  agent_id: string
  project_id: string
  issue_id: string
  issue_title: string
  issue_description: string
  enabled: boolean
  last_fired_at: string | null
  next_run_at: string | null
  created_at: string
}

export interface AutopilotInput {
  name: string
  trigger_type: Autopilot['trigger_type']
  cron_spec: string
  webhook_id: string
  action_type: Autopilot['action_type']
  agent_id: string
  project_id: string
  issue_id: string
  issue_title: string
  issue_description: string
  enabled: boolean
}

export async function listAutopilots(): Promise<Autopilot[]> {
  const res = await apiFetch<{ autopilots: Autopilot[] }>('/autopilots')
  return res.autopilots ?? []
}

export async function createAutopilot(input: AutopilotInput): Promise<Autopilot> {
  const res = await apiFetch<{ autopilot: Autopilot }>('/autopilots', {
    method: 'POST',
    body: input,
  })
  return res.autopilot
}

export async function updateAutopilot(id: string, input: AutopilotInput): Promise<Autopilot> {
  const res = await apiFetch<{ autopilot: Autopilot }>(`/autopilots/${id}`, {
    method: 'PUT',
    body: input,
  })
  return res.autopilot
}

export async function deleteAutopilot(id: string): Promise<void> {
  await apiFetch(`/autopilots/${id}`, { method: 'DELETE' })
}

export async function triggerAutopilot(id: string): Promise<void> {
  await apiFetch(`/autopilots/${id}/trigger`, { method: 'POST' })
}
