import { useEffect, useMemo, useState } from 'react'
import { getChangeByIssue, listArtifacts, listArtifactVersions, type Artifact } from '../api/workflow'
import { renderMarkdown } from '../lib/markdown'

const KIND_LABELS: Record<string, string> = {
  proposal: '提案',
  specs: '规格',
  design: '设计',
  tasks: '任务',
  verify: '验证',
}

const KIND_ORDER = ['proposal', 'specs', 'design', 'tasks', 'verify']

export function ArtifactViewer({ issueId }: { issueId: string }) {
  const [changeId, setChangeId] = useState<string | null>(null)
  const [latest, setLatest] = useState<Artifact[] | null>(null)
  const [kind, setKind] = useState<string | null>(null)
  const [versions, setVersions] = useState<Artifact[]>([])
  const [version, setVersion] = useState<number | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setChangeId(null)
    setLatest(null)
    setError('')
    getChangeByIssue(issueId)
      .then((c) => {
        if (!cancelled) setChangeId(c.id)
      })
      .catch(() => {
        if (!cancelled) setChangeId('')
      })
    return () => {
      cancelled = true
    }
  }, [issueId])

  useEffect(() => {
    if (!changeId) return
    let cancelled = false
    listArtifacts(changeId)
      .then((list) => {
        if (cancelled) return
        const sorted = [...list].sort(
          (a, b) => KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind),
        )
        setLatest(sorted)
        setKind(sorted[0]?.kind ?? null)
      })
      .catch(() => {
        if (!cancelled) setError('产物加载失败')
      })
    return () => {
      cancelled = true
    }
  }, [changeId])

  useEffect(() => {
    if (!changeId || !kind) return
    let cancelled = false
    listArtifactVersions(changeId, kind)
      .then((list) => {
        if (cancelled) return
        setVersions(list)
        setVersion(list[0]?.version ?? null)
      })
      .catch(() => {
        if (!cancelled) setError('产物加载失败')
      })
    return () => {
      cancelled = true
    }
  }, [changeId, kind])

  const selected = useMemo(
    () => versions.find((a) => a.version === version) ?? null,
    [versions, version],
  )

  if (error) return <div data-testid="artifacts-error">{error}</div>
  if (latest === null) return null
  if (latest.length === 0) return <div data-testid="artifacts-empty" />

  return (
    <section className="artifact-viewer" data-testid="artifact-viewer">
      <div role="tablist" className="artifact-tabs">
        {latest.map((a) => (
          <button
            key={a.kind}
            role="tab"
            aria-selected={a.kind === kind}
            onClick={() => setKind(a.kind)}
          >
            {KIND_LABELS[a.kind] ?? a.kind}
          </button>
        ))}
      </div>
      {versions.length > 1 && (
        <label>
          版本{' '}
          <select
            aria-label="版本"
            data-testid="artifact-version"
            value={version ?? ''}
            onChange={(e) => setVersion(Number(e.target.value))}
          >
            {versions.map((a) => (
              <option key={a.version} value={a.version}>
                v{a.version}
              </option>
            ))}
          </select>
        </label>
      )}
      {versions.length === 1 && (
        <span data-testid="artifact-version">v{versions[0].version}</span>
      )}
      {selected && (
        <div
          className="artifact-content markdown-body"
          data-testid="artifact-content"
          dangerouslySetInnerHTML={{ __html: renderMarkdown(selected.content) }}
        />
      )}
    </section>
  )
}
