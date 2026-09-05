import { useCallback, useEffect, useState } from 'react'
import { getRun, listRuns, type Run, type RunLog } from '../api/runs'
import { ApiError } from '../api/client'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

const STATUS_LABELS: Record<string, string> = {
  queued: '排队中',
  running: '运行中',
  done: '已完成',
  failed: '失败',
}

const TRIGGER_LABELS: Record<string, string> = {
  manual: '手动触发',
  assigned: '指派触发',
  status_changed: '状态变更触发',
  mention: '提及触发',
}

function formatTime(value: string | null): string {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}

function formatDuration(run: Run): string {
  if (!run.started_at) return '—'
  const end = run.finished_at ? new Date(run.finished_at) : new Date()
  const seconds = Math.max(0, Math.round((end.getTime() - new Date(run.started_at).getTime()) / 1000))
  return `${seconds}s`
}

function RunLogs({ runId }: { runId: string }) {
  const [logs, setLogs] = useState<RunLog[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    getRun(runId)
      .then((res) => {
        if (!cancelled) setLogs(res.logs ?? [])
      })
      .catch((err) => {
        if (!cancelled) setError(errorMessage(err, '加载执行日志失败'))
      })
    return () => {
      cancelled = true
    }
  }, [runId])

  if (error) return <p role="alert">{error}</p>
  if (logs === null) return <p>日志加载中…</p>
  if (logs.length === 0) return <p>本次运行没有日志。</p>
  return (
    <pre className="run-logs" data-testid={`run-logs-${runId}`}>
      {logs.map((l) => (
        <div key={l.seq} data-testid={`run-log-${runId}-${l.seq}`}>
          [{l.kind}] {l.content}
        </div>
      ))}
    </pre>
  )
}

export function RunHistory({ issueId }: { issueId: string }) {
  const [runs, setRuns] = useState<Run[] | null>(null)
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState('')

  const load = useCallback(() => {
    listRuns({ issue_id: issueId })
      .then(setRuns)
      .catch((err) => setError(errorMessage(err, '加载运行历史失败')))
  }, [issueId])

  useEffect(() => {
    setRuns(null)
    setExpandedId('')
    load()
  }, [load])

  const toggle = (runId: string) => {
    setExpandedId((cur) => (cur === runId ? '' : runId))
  }

  return (
    <div data-testid="run-history">
      <h3>运行历史</h3>
      {error && (
        <p role="alert" data-testid="run-history-error">
          {error}
        </p>
      )}
      {runs && runs.length === 0 && <p>暂无运行记录。</p>}
      <ul className="run-list">
        {(runs ?? []).map((r) => (
          <li key={r.id} data-testid={`run-${r.id}`}>
            <div className="run-summary">
              <span className="badge">{TRIGGER_LABELS[r.trigger] ?? r.trigger}</span>
              <span data-testid={`run-status-${r.id}`} className="badge">
                {STATUS_LABELS[r.status] ?? r.status}
              </span>
              <span>
                {formatTime(r.started_at)} → {formatTime(r.finished_at)}（{formatDuration(r)}）
              </span>
              <button data-testid={`run-toggle-${r.id}`} onClick={() => toggle(r.id)}>
                {expandedId === r.id ? '收起日志' : '查看日志'}
              </button>
            </div>
            {r.error && (
              <p role="alert" data-testid={`run-error-${r.id}`}>
                {r.error}
              </p>
            )}
            {expandedId === r.id && <RunLogs runId={r.id} />}
          </li>
        ))}
      </ul>
    </div>
  )
}
