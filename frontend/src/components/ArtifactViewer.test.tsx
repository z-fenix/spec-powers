import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ArtifactViewer } from './ArtifactViewer'
import * as api from '../api/workflow'
import type { Artifact, Change } from '../api/workflow'

vi.mock('../api/workflow', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workflow')>()
  return {
    ...actual,
    getChangeByIssue: vi.fn(),
    listArtifacts: vi.fn(),
    listArtifactVersions: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function artifact(kind: string, version: number, content: string): Artifact {
  return {
    id: `a-${kind}-${version}`,
    change_id: 'c1',
    kind,
    version,
    content,
    created_by: 'u1',
    created_at: '2026-09-04T00:00:00Z',
  }
}

const change: Change = {
  id: 'c1',
  project_id: 'p1',
  issue_id: 'i1',
  phase: 'tasks',
  status: 'active',
  created_by: 'u1',
  created_at: '2026-09-04T00:00:00Z',
  updated_at: '2026-09-04T01:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.getChangeByIssue.mockReset().mockResolvedValue(change)
  mocked.listArtifacts.mockReset()
  mocked.listArtifactVersions.mockReset()
})

describe('ArtifactViewer', () => {
  it('shows a tab per artifact kind and renders the latest content as markdown', async () => {
    mocked.listArtifacts.mockResolvedValueOnce([
      artifact('proposal', 2, '# 提案标题\n\n第一段'),
      artifact('specs', 1, '## 规格'),
    ])
    mocked.listArtifactVersions.mockResolvedValueOnce([
      artifact('proposal', 2, '# 提案标题\n\n第一段'),
      artifact('proposal', 1, '# 旧提案'),
    ])

    render(<ArtifactViewer issueId="i1" />)

    expect(await screen.findByRole('tab', { name: '提案' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '规格' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: '提案标题' })).toBeInTheDocument()
    expect(screen.getByTestId('artifact-version')).toHaveTextContent('v2')
  })

  it('switches between versions', async () => {
    const user = userEvent.setup()
    mocked.listArtifacts.mockResolvedValueOnce([artifact('proposal', 2, '# 新版')])
    mocked.listArtifactVersions
      .mockResolvedValueOnce([artifact('proposal', 2, '# 新版'), artifact('proposal', 1, '# 旧版')])
      .mockResolvedValueOnce([artifact('proposal', 2, '# 新版'), artifact('proposal', 1, '# 旧版')])

    render(<ArtifactViewer issueId="i1" />)

    await screen.findByRole('heading', { level: 1, name: '新版' })
    await user.selectOptions(screen.getByLabelText('版本'), '1')

    expect(await screen.findByRole('heading', { level: 1, name: '旧版' })).toBeInTheDocument()
    expect(mocked.listArtifactVersions).toHaveBeenCalledTimes(1)
  })

  it('switches between kinds', async () => {
    const user = userEvent.setup()
    mocked.listArtifacts.mockResolvedValueOnce([
      artifact('proposal', 1, '# 提案'),
      artifact('tasks', 1, '# 任务'),
    ])
    mocked.listArtifactVersions.mockImplementation(async (_changeID, kind) => [
      artifact(kind, 1, `# ${kind}`),
    ])

    render(<ArtifactViewer issueId="i1" />)

    await screen.findByRole('heading', { level: 1, name: 'proposal' })
    await user.click(screen.getByRole('tab', { name: '任务' }))

    expect(await screen.findByRole('heading', { level: 1, name: 'tasks' })).toBeInTheDocument()
  })

  it('renders nothing when there are no artifacts', async () => {
    mocked.listArtifacts.mockResolvedValueOnce([])

    render(<ArtifactViewer issueId="i1" />)

    expect(await screen.findByTestId('artifacts-empty')).toBeInTheDocument()
  })

  it('sanitizes raw html in artifact content', async () => {
    mocked.listArtifacts.mockResolvedValueOnce([
      artifact('proposal', 1, '<p>safe</p><script>alert(1)</script>'),
    ])
    mocked.listArtifactVersions.mockResolvedValueOnce([
      artifact('proposal', 1, '<p>safe</p><script>alert(1)</script>'),
    ])

    const { container } = render(<ArtifactViewer issueId="i1" />)

    await screen.findByText('safe')
    expect(container.querySelector('script')).toBeNull()
  })
})
