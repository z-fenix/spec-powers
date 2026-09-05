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

export interface UsageTotals {
  calls: number
  prompt_tokens: number
  completion_tokens: number
}

export interface IssueUsageRow extends UsageTotals {
  issue_id: string
  title: string
}

export async function getRun(runID: string): Promise<{ run: Run; logs: RunLog[] }> {
  return apiFetch<{ run: Run; logs: RunLog[] }>(`/runs/${encodeURIComponent(runID)}`)
}

export async function getIssueUsage(issueID: string): Promise<UsageTotals> {
  const res = await apiFetch<{ issue_id: string; usage: UsageTotals }>(
    `/runs/usage?issue_id=${encodeURIComponent(issueID)}`,
  )
  return res.usage
}

export async function getProjectUsage(projectID: string): Promise<IssueUsageRow[]> {
  const res = await apiFetch<{ project_id: string; usage: IssueUsageRow[] }>(
    `/runs/usage?project_id=${encodeURIComponent(projectID)}`,
  )
  return res.usage ?? []
}
