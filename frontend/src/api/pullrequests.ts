import { apiFetch } from './client'

export type PullRequestState = 'open' | 'merged' | 'closed'

export interface PullRequest {
  id: string
  project_id: string
  repo: string
  number: number
  title: string
  body: string
  head_branch: string
  state: PullRequestState | string
  merged_at: string
  issue_keys: string[]
  created_at: string
  updated_at: string
}

export interface PullRequestInput {
  repo: string
  number: number
  title: string
  body?: string
  head_branch?: string
  state?: PullRequestState | string
}

export function pullRequestPath(projectId: string, prId?: string): string {
  const base = `/projects/${projectId}/pullrequests`
  return prId ? `${base}/${prId}` : base
}

export async function upsertPullRequest(
  projectId: string,
  input: PullRequestInput,
): Promise<PullRequest> {
  const res = await apiFetch<{ pull_request: PullRequest }>(pullRequestPath(projectId), {
    method: 'POST',
    body: input,
  })
  return res.pull_request
}

export async function updatePullRequestState(
  projectId: string,
  prId: string,
  state: PullRequestState | string,
): Promise<PullRequest> {
  const res = await apiFetch<{ pull_request: PullRequest }>(pullRequestPath(projectId, prId), {
    method: 'PATCH',
    body: { state },
  })
  return res.pull_request
}

export async function listIssuePullRequests(
  projectId: string,
  issueId: string,
): Promise<PullRequest[]> {
  const res = await apiFetch<{ pull_requests: PullRequest[] }>(
    `/projects/${projectId}/issues/${issueId}/pullrequests`,
  )
  return res.pull_requests ?? []
}
