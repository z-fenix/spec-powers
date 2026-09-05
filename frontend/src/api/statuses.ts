import { apiFetch } from './client'
import type { StatusEntry } from '../lib/status'

export type WorkspaceStatus = StatusEntry

export async function listStatuses(projectId: string): Promise<WorkspaceStatus[]> {
  const res = await apiFetch<{ statuses: WorkspaceStatus[] }>(
    `/projects/${projectId}/issues/statuses`,
  )
  return res.statuses
}

export async function upsertStatus(
  projectId: string,
  name: string,
  category: string,
): Promise<WorkspaceStatus[]> {
  const res = await apiFetch<{ statuses: WorkspaceStatus[] }>(
    `/projects/${projectId}/issues/statuses`,
    { method: 'PUT', body: { name, category } },
  )
  return res.statuses
}

export async function deleteStatus(projectId: string, name: string): Promise<WorkspaceStatus[]> {
  const res = await apiFetch<{ statuses: WorkspaceStatus[] }>(
    `/projects/${projectId}/issues/statuses/${encodeURIComponent(name)}`,
    { method: 'DELETE' },
  )
  return res.statuses
}
