import { useEffect, useState } from 'react'
import { getIssueUsage, getProjectUsage, type IssueUsageRow, type UsageTotals } from '../api/runs'

export function formatTokens(n: number): string {
  return n.toLocaleString('en-US')
}

export function IssueUsagePanel({ issueId }: { issueId: string }) {
  const [usage, setUsage] = useState<UsageTotals | null>(null)

  useEffect(() => {
    let cancelled = false
    getIssueUsage(issueId)
      .then((u) => {
        if (!cancelled) setUsage(u)
      })
      .catch(() => {
        if (!cancelled) setUsage({ calls: 0, prompt_tokens: 0, completion_tokens: 0 })
      })
    return () => {
      cancelled = true
    }
  }, [issueId])

  if (!usage) return null
  return (
    <div className="usage-panel" data-testid="issue-usage">
      <h3>LLM 用量</h3>
      <p className="usage-totals">
        <span data-testid="usage-calls">{usage.calls}</span> 次调用 · prompt{' '}
        <span data-testid="usage-prompt">{formatTokens(usage.prompt_tokens)}</span> · completion{' '}
        <span data-testid="usage-completion">{formatTokens(usage.completion_tokens)}</span>
      </p>
    </div>
  )
}

export function ProjectUsagePanel({ projectId }: { projectId: string }) {
  const [rows, setRows] = useState<IssueUsageRow[] | null>(null)

  useEffect(() => {
    let cancelled = false
    getProjectUsage(projectId)
      .then((list) => {
        if (!cancelled) setRows(list)
      })
      .catch(() => {
        if (!cancelled) setRows([])
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  if (rows === null) return null
  return (
    <div className="detail-section" data-testid="project-usage">
      <h3 className="section-title">LLM 用量</h3>
      {rows.length === 0 && <p className="empty-state">还没有记录到 LLM 用量。</p>}
      <ul className="project-list">
        {rows.map((u) => (
          <li key={u.issue_id} className="collection-row" data-testid={`usage-row-${u.issue_id}`}>
            <div className="collection-row-main">
              <strong>{u.title}</strong>
              <span className="usage-totals">
                {u.calls} 次调用 · prompt {formatTokens(u.prompt_tokens)} · completion{' '}
                {formatTokens(u.completion_tokens)}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
