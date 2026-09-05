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
