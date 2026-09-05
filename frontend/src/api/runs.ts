import { apiFetch } from './client'

export interface Run {
  id: string
  agent_id: string
  issue_id: string
  trigger: string
  status: string
  error: string
  created_at: string
  started_at: string | null
  finished_at: string | null
}

export interface RunLog {
  seq: number
  kind: string
  content: string
}

export async function getRun(runID: string): Promise<{ run: Run; logs: RunLog[] }> {
  return apiFetch<{ run: Run; logs: RunLog[] }>(`/runs/${encodeURIComponent(runID)}`)
}

export interface RunFilter {
  issue_id?: string
  agent_id?: string
  status?: string
}

export async function listRuns(filter?: RunFilter): Promise<Run[]> {
  const params = new URLSearchParams()
  if (filter?.issue_id) params.set('issue_id', filter.issue_id)
  if (filter?.agent_id) params.set('agent_id', filter.agent_id)
  if (filter?.status) params.set('status', filter.status)
  const qs = params.toString()
  const res = await apiFetch<{ runs: Run[] }>(qs ? `/runs?${qs}` : '/runs')
  return res.runs ?? []
}
