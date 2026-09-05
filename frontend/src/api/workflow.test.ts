import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  getChangeByIssue,
  getChange,
  listArtifacts,
  getArtifact,
  listArtifactVersions,
  getGuard,
  type Change,
  type Artifact,
  type GuardReport,
} from './workflow'
import { apiFetch } from './client'

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

const change: Change = {
  id: 'c1',
  project_id: 'p1',
  issue_id: 'i1',
  phase: 'design',
  status: 'active',
  created_by: 'u1',
  created_at: '2026-09-04T00:00:00Z',
  updated_at: '2026-09-04T01:00:00Z',
}

const artifact: Artifact = {
  id: 'a1',
  change_id: 'c1',
  kind: 'proposal',
  version: 1,
  content: '# Proposal',
  created_by: 'u1',
  run_id: null,
  created_at: '2026-09-04T00:00:00Z',
}

const guard: GuardReport = {
  change_id: 'c1',
  phase: 'design',
  next_phase: 'tasks',
  phase_legal: true,
  handoff_fresh: true,
  verify_passed: false,
  can_advance: true,
  can_archive: false,
  reasons: [],
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

describe('workflow api', () => {
  it('gets the change running for an issue', async () => {
    mockedFetch.mockResolvedValueOnce({ change })
    const res = await getChangeByIssue('i1')

    expect(apiFetch).toHaveBeenCalledWith('/changes?issue_id=i1')
    expect(res).toEqual(change)
  })

  it('gets one change', async () => {
    mockedFetch.mockResolvedValueOnce({ change })
    const res = await getChange('c1')

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1')
    expect(res).toEqual(change)
  })

  it('lists the latest artifact per kind', async () => {
    mockedFetch.mockResolvedValueOnce({ artifacts: [artifact] })
    const res = await listArtifacts('c1')

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1/artifacts')
    expect(res).toEqual([artifact])
  })

  it('gets one artifact kind at the latest version', async () => {
    mockedFetch.mockResolvedValueOnce({ artifact })
    const res = await getArtifact('c1', 'proposal')

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1/artifacts/proposal')
    expect(res).toEqual(artifact)
  })

  it('gets one artifact kind at an explicit version', async () => {
    mockedFetch.mockResolvedValueOnce({ artifact })
    await getArtifact('c1', 'proposal', 2)

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1/artifacts/proposal?version=2')
  })

  it('lists every version of one kind', async () => {
    mockedFetch.mockResolvedValueOnce({ artifacts: [artifact] })
    const res = await listArtifactVersions('c1', 'specs')

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1/artifacts/specs/versions')
    expect(res).toEqual([artifact])
  })

  it('gets the guard gate report', async () => {
    mockedFetch.mockResolvedValueOnce({ guard })
    const res = await getGuard('c1')

    expect(apiFetch).toHaveBeenCalledWith('/changes/c1/guard')
    expect(res).toEqual(guard)
  })
})
