import { apiFetch } from './client'

export type ArtifactKind = 'proposal' | 'specs' | 'design' | 'tasks' | 'verify'

export interface Change {
  id: string
  project_id: string
  issue_id: string
  phase: string
  status: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface Artifact {
  id: string
  change_id: string
  kind: ArtifactKind | string
  version: number
  content: string
  created_by: string
  run_id: string | null
  created_at: string
}

export interface GuardReport {
  change_id: string
  phase: string
  next_phase: string
  phase_legal: boolean
  handoff_fresh: boolean
  verify_passed: boolean
  can_advance: boolean
  can_archive: boolean
  reasons: string[]
}

export async function getChangeByIssue(issueID: string): Promise<Change> {
  const res = await apiFetch<{ change: Change }>(`/changes?issue_id=${encodeURIComponent(issueID)}`)
  return res.change
}

export async function getChange(changeID: string): Promise<Change> {
  const res = await apiFetch<{ change: Change }>(`/changes/${encodeURIComponent(changeID)}`)
  return res.change
}

export async function listArtifacts(changeID: string): Promise<Artifact[]> {
  const res = await apiFetch<{ artifacts: Artifact[] }>(`/changes/${encodeURIComponent(changeID)}/artifacts`)
  return res.artifacts
}

export async function getArtifact(changeID: string, kind: string, version?: number): Promise<Artifact> {
  const base = `/changes/${encodeURIComponent(changeID)}/artifacts/${encodeURIComponent(kind)}`
  const path = version !== undefined ? `${base}?version=${version}` : base
  const res = await apiFetch<{ artifact: Artifact }>(path)
  return res.artifact
}

export async function listArtifactVersions(changeID: string, kind: string): Promise<Artifact[]> {
  const res = await apiFetch<{ artifacts: Artifact[] }>(
    `/changes/${encodeURIComponent(changeID)}/artifacts/${encodeURIComponent(kind)}/versions`,
  )
  return res.artifacts
}

export async function getGuard(changeID: string): Promise<GuardReport> {
  const res = await apiFetch<{ guard: GuardReport }>(`/changes/${encodeURIComponent(changeID)}/guard`)
  return res.guard
}
