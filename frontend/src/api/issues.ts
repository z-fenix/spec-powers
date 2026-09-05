import { apiFetch, getToken, ApiError } from './client'

export type IssueStatus =
  | 'todo'
  | 'in_progress'
  | 'in_review'
  | 'done'
  | 'blocked'
  | 'backlog'
  | 'cancelled'

export interface Issue {
  id: string
  project_id: string
  parent_id: string
  title: string
  description: string
  status: IssueStatus | string
  priority: string
  assignee_id: string
  due_date: string
  labels: string[]
  stage: number
  position: number
  created_by: string
}

export interface IssueComment {
  id: string
  issue_id: string
  parent_id: string
  author_id: string
  content: string
  created_at: string
}

export interface Attachment {
  id: string
  issue_id: string
  comment_id: string
  file_name: string
  size_bytes: number
  content_type: string
  uploaded_by: string
  created_at: string
}

export interface MetadataEntry {
  key: string
  value: string
  type: string
  updated_at: string
}

export interface IssueInput {
  title: string
  description?: string
  priority?: string
  assignee_id?: string
  due_date?: string
  labels?: string[]
  parent_id?: string
  stage?: number
}

export interface IssuePatch {
  title?: string
  description?: string
  priority?: string
  assignee_id?: string
  due_date?: string
  labels?: string[]
  parent_id?: string
  stage?: number
  position?: number
}

export interface IssueFilter {
  status?: string
  stage?: number
  parent?: string
  query?: string
}

export interface IssueEvent {
  id: string
  issue_id: string
  actor_id: string
  field: string
  old_value: string
  new_value: string
  created_at: string
}

function issuePath(projectId: string, issueId?: string): string {
  const base = `/projects/${projectId}/issues`
  return issueId ? `${base}/${issueId}` : base
}

function withQuery(path: string, filter?: IssueFilter): string {
  if (!filter) return path
  const params = new URLSearchParams()
  if (filter.status) params.set('status', filter.status)
  if (filter.stage !== undefined) params.set('stage', String(filter.stage))
  if (filter.parent) params.set('parent', filter.parent)
  if (filter.query) params.set('q', filter.query)
  const qs = params.toString()
  return qs ? `${path}?${qs}` : path
}

export async function listIssues(projectId: string, filter?: IssueFilter): Promise<Issue[]> {
  const res = await apiFetch<{ issues: Issue[] }>(withQuery(issuePath(projectId), filter))
  return res.issues ?? []
}

export async function getIssue(projectId: string, issueId: string): Promise<Issue> {
  const res = await apiFetch<{ issue: Issue }>(issuePath(projectId, issueId))
  return res.issue
}

export async function createIssue(projectId: string, input: IssueInput): Promise<Issue> {
  const res = await apiFetch<{ issue: Issue }>(issuePath(projectId), { method: 'POST', body: input })
  return res.issue
}

export async function updateIssue(
  projectId: string,
  issueId: string,
  patch: IssuePatch,
): Promise<Issue> {
  const res = await apiFetch<{ issue: Issue }>(issuePath(projectId, issueId), {
    method: 'PATCH',
    body: patch,
  })
  return res.issue
}

export async function deleteIssue(projectId: string, issueId: string): Promise<void> {
  await apiFetch<void>(issuePath(projectId, issueId), { method: 'DELETE' })
}

export async function transitionIssue(
  projectId: string,
  issueId: string,
  status: string,
): Promise<Issue> {
  const res = await apiFetch<{ issue: Issue }>(`${issuePath(projectId, issueId)}/status`, {
    method: 'POST',
    body: { status },
  })
  return res.issue
}

export async function listChildren(projectId: string, issueId: string): Promise<Issue[]> {
  const res = await apiFetch<{ issues: Issue[] }>(`${issuePath(projectId, issueId)}/children`)
  return res.issues ?? []
}

export async function listIssueEvents(
  projectId: string,
  issueId: string,
): Promise<IssueEvent[]> {
  const res = await apiFetch<{ events: IssueEvent[] }>(
    `${issuePath(projectId, issueId)}/events`,
  )
  return res.events ?? []
}

export async function listComments(projectId: string, issueId: string): Promise<IssueComment[]> {
  const res = await apiFetch<{ comments: IssueComment[] }>(
    `${issuePath(projectId, issueId)}/comments`,
  )
  return res.comments ?? []
}

export async function addComment(
  projectId: string,
  issueId: string,
  content: string,
  parentId?: string,
): Promise<IssueComment> {
  const res = await apiFetch<{ comment: IssueComment }>(
    `${issuePath(projectId, issueId)}/comments`,
    { method: 'POST', body: { content, parent_id: parentId ?? '' } },
  )
  return res.comment
}

export async function listAttachments(
  projectId: string,
  issueId: string,
): Promise<Attachment[]> {
  const res = await apiFetch<{ attachments: Attachment[] }>(
    `${issuePath(projectId, issueId)}/attachments`,
  )
  return res.attachments ?? []
}

export async function uploadAttachment(
  projectId: string,
  issueId: string,
  file: File,
): Promise<Attachment> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const form = new FormData()
  form.append('file', file)
  const BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api/v1'
  const res = await fetch(`${BASE}${issuePath(projectId, issueId)}/attachments`, {
    method: 'POST',
    headers,
    body: form,
  })
  const payload = (await res.json().catch(() => null)) as { attachment?: Attachment } | null
  if (!res.ok) {
    const err = (payload as { error?: { code?: string; message?: string } } | null)?.error
    throw new ApiError(res.status, err?.code ?? 'unknown', err?.message ?? 'upload failed')
  }
  return payload!.attachment!
}

export async function downloadAttachment(
  projectId: string,
  issueId: string,
  attachmentId: string,
): Promise<Blob> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api/v1'
  const res = await fetch(`${BASE}${issuePath(projectId, issueId)}/attachments/${attachmentId}`, {
    headers,
  })
  if (!res.ok) {
    throw new ApiError(res.status, 'download_failed', `download failed with status ${res.status}`)
  }
  return res.blob()
}

export async function listMetadata(
  projectId: string,
  issueId: string,
): Promise<MetadataEntry[]> {
  const res = await apiFetch<{ metadata: MetadataEntry[] }>(
    `${issuePath(projectId, issueId)}/metadata`,
  )
  return res.metadata ?? []
}

export async function setMetadata(
  projectId: string,
  issueId: string,
  key: string,
  value: string,
  type?: string,
): Promise<MetadataEntry> {
  const res = await apiFetch<{ entry: MetadataEntry }>(
    `${issuePath(projectId, issueId)}/metadata/${encodeURIComponent(key)}`,
    { method: 'PUT', body: { value, type: type ?? 'string' } },
  )
  return res.entry
}

export async function deleteMetadata(
  projectId: string,
  issueId: string,
  key: string,
): Promise<void> {
  await apiFetch<void>(`${issuePath(projectId, issueId)}/metadata/${encodeURIComponent(key)}`, {
    method: 'DELETE',
  })
}

export interface Subscriber {
  user_id: string
  display_name: string
  email: string
}

export async function listSubscribers(
  projectId: string,
  issueId: string,
): Promise<Subscriber[]> {
  const res = await apiFetch<{ subscribers: Subscriber[] }>(
    `${issuePath(projectId, issueId)}/subscribers`,
  )
  return res.subscribers ?? []
}

export async function addSubscriber(
  projectId: string,
  issueId: string,
  email: string,
): Promise<Subscriber[]> {
  const res = await apiFetch<{ subscribers: Subscriber[] }>(
    `${issuePath(projectId, issueId)}/subscribers`,
    { method: 'POST', body: { email } },
  )
  return res.subscribers ?? []
}

export async function removeSubscriber(
  projectId: string,
  issueId: string,
  userId: string,
): Promise<void> {
  await apiFetch<void>(
    `${issuePath(projectId, issueId)}/subscribers/${encodeURIComponent(userId)}`,
    { method: 'DELETE' },
  )
}
