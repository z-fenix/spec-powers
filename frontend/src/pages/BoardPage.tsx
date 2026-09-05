import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  createIssue,
  listIssues,
  transitionIssue,
  updateIssue,
  type Issue,
} from '../api/issues'
import { getChangeByIssue, listArtifacts, type Artifact } from '../api/workflow'
import { getRun, type RunLog } from '../api/runs'
import { ApiError } from '../api/client'
import {
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  isOverdue,
  parseLabels,
} from '../lib/status'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

const KIND_LABELS: Record<string, string> = {
  proposal: '提案',
  specs: '规格',
  design: '设计',
  tasks: '任务',
  verify: '验证',
}

interface IssueCardProps {
  issue: Issue
  projectId: string
  onTransition: (issueId: string, status: string) => void
  onPriorityChange: (issueId: string, priority: string) => void
}

function IssueCard({ issue, projectId, onTransition, onPriorityChange }: IssueCardProps) {
  const [showArtifacts, setShowArtifacts] = useState(false)
  const overdue = isOverdue(issue.due_date, issue.status)
  return (
    <div className="issue-card" data-testid={`card-${issue.id}`}>
      <Link to={`/projects/${projectId}/issues/${issue.id}`}>{issue.title}</Link>
      {issue.stage > 0 && <span className="badge">S{issue.stage}</span>}
      <select
        aria-label="优先级"
        data-testid={`priority-${issue.id}`}
        value={issue.priority}
        onChange={(e) => onPriorityChange(issue.id, e.target.value)}
      >
        {PRIORITIES.map((p) => (
          <option key={p} value={p}>
            {PRIORITY_LABELS[p]}
          </option>
        ))}
      </select>
      {issue.labels.length > 0 && (
        <span className="card-labels" data-testid={`labels-${issue.id}`}>
          {issue.labels.map((l) => (
            <span key={l} className="badge label">
              {l}
            </span>
          ))}
        </span>
      )}
      {issue.due_date && (
        <span className={overdue ? 'due-date overdue' : 'due-date'} data-testid={`due-${issue.id}`}>
          截止: {issue.due_date.slice(0, 10)}
          {overdue && '（已逾期）'}
        </span>
      )}
      <select
        aria-label="状态"
        data-testid={`status-${issue.id}`}
        value={issue.status}
        onChange={(e) => onTransition(issue.id, e.target.value)}
      >
        {STATUSES.map((s) => (
          <option key={s} value={s}>
            {STATUS_LABELS[s]}
          </option>
        ))}
      </select>
      <button
        data-testid={`toggle-artifacts-${issue.id}`}
        onClick={() => setShowArtifacts((v) => !v)}
      >
        产物
      </button>
      {showArtifacts && <IssueArtifacts issueId={issue.id} />}
    </div>
  )
}

type LogsState = Record<string, RunLog[] | 'loading'>

// IssueArtifacts lists the issue's workflow artifacts, each with its
// producing run's execution logs when one is linked.
function IssueArtifacts({ issueId }: { issueId: string }) {
  const [state, setState] = useState<'loading' | 'empty' | 'ready'>('loading')
  const [artifacts, setArtifacts] = useState<Artifact[]>([])
  const [logs, setLogs] = useState<LogsState>({})

  useEffect(() => {
    let cancelled = false
    getChangeByIssue(issueId)
      .then((c) => listArtifacts(c.id))
      .then((list) => {
        if (cancelled) return
        setArtifacts(list)
        setState(list.length === 0 ? 'empty' : 'ready')
      })
      .catch(() => {
        // no change for the issue means no workflow artifacts
        if (!cancelled) setState('empty')
      })
    return () => {
      cancelled = true
    }
  }, [issueId])

  const onToggleLogs = (a: Artifact) => {
    if (!a.run_id) return
    const open = logs[a.id] !== undefined
    if (open) {
      setLogs((prev) => {
        const next = { ...prev }
        delete next[a.id]
        return next
      })
      return
    }
    setLogs((prev) => ({ ...prev, [a.id]: 'loading' }))
    getRun(a.run_id)
      .then((res) => setLogs((prev) => ({ ...prev, [a.id]: res.logs })))
      .catch(() =>
        setLogs((prev) => {
          const next = { ...prev }
          delete next[a.id]
          return next
        }),
      )
  }

  if (state === 'loading') return <p data-testid={`artifacts-loading-${issueId}`}>加载中…</p>
  if (state === 'empty')
    return <p data-testid={`artifacts-empty-${issueId}`}>暂无流程产物</p>

  return (
    <ul className="board-artifacts" data-testid={`artifacts-${issueId}`}>
      {artifacts.map((a) => {
        const logState = logs[a.id]
        return (
          <li key={a.id} data-testid={`artifact-${a.id}`}>
            <span className="badge">{KIND_LABELS[a.kind] ?? a.kind}</span> v{a.version}
            {a.run_id && (
              <button data-testid={`logs-${a.id}`} onClick={() => onToggleLogs(a)}>
                {logState !== undefined ? '收起日志' : '执行日志'}
              </button>
            )}
            {logState === 'loading' && <p>日志加载中…</p>}
            {Array.isArray(logState) && (
              <ol className="run-logs" data-testid={`logs-${a.id}-list`}>
                {logState.map((l) => (
                  <li key={l.seq}>
                    <span className="badge">{l.kind}</span>
                    <pre>{l.content}</pre>
                  </li>
                ))}
              </ol>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function BoardPage() {
  const { id = '' } = useParams()
  const [issues, setIssues] = useState<Issue[] | null>(null)
  const [error, setError] = useState('')
  const [view, setView] = useState<'board' | 'list'>('board')
  const [statusFilter, setStatusFilter] = useState('')
  const [stageFilter, setStageFilter] = useState('')
  const [labelFilter, setLabelFilter] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [stage, setStage] = useState('')
  const [priority, setPriority] = useState('none')
  const [labels, setLabels] = useState('')
  const [dueDate, setDueDate] = useState('')

  const load = useCallback(
    (filter?: { status?: string; stage?: number; label?: string }) => {
      return listIssues(id, filter)
        .then(setIssues)
        .catch((err) => setError(errorMessage(err, '加载失败')))
    },
    [id],
  )

  useEffect(() => {
    setIssues(null)
    setError('')
    load()
  }, [load])

  const onFilter = (status: string, stage: string, label: string) => {
    setStatusFilter(status)
    setStageFilter(stage)
    setLabelFilter(label)
    const filter: { status?: string; stage?: number; label?: string } = {}
    if (status) filter.status = status
    if (stage) filter.stage = Number(stage)
    if (label) filter.label = label
    load(filter)
  }

  const onTransition = (issueId: string, status: string) => {
    setError('')
    transitionIssue(id, issueId, status)
      .then(() => load())
      .catch((err) => setError(errorMessage(err, '状态流转失败')))
  }

  const onPriorityChange = (issueId: string, prio: string) => {
    setError('')
    updateIssue(id, issueId, { priority: prio })
      .then(() => load())
      .catch((err) => setError(errorMessage(err, '优先级更新失败')))
  }

  const onCreate = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    createIssue(id, {
      title,
      description,
      stage: stage ? Number(stage) : undefined,
      priority,
      labels: labels ? parseLabels(labels) : undefined,
      due_date: dueDate || undefined,
    })
      .then(() => {
        setTitle('')
        setDescription('')
        setStage('')
        setPriority('none')
        setLabels('')
        setDueDate('')
        setShowCreate(false)
        return load()
      })
      .catch((err) => setError(errorMessage(err, '创建失败')))
  }

  const visible = issues ?? []
  const byStage = (stage: number) => visible.filter((i) => i.stage === stage)

  return (
    <section data-testid="board">
      <h2>Issue 看板</h2>
      {error && (
        <p role="alert" data-testid="board-error">
          {error}
        </p>
      )}
      <div className="board-toolbar">
        <select
          aria-label="状态筛选"
          data-testid="filter-status"
          value={statusFilter}
          onChange={(e) => onFilter(e.target.value, stageFilter, labelFilter)}
        >
          <option value="">全部状态</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {STATUS_LABELS[s]}
            </option>
          ))}
        </select>
        <input
          aria-label="Stage 筛选"
          data-testid="filter-stage"
          type="number"
          min={0}
          placeholder="Stage"
          value={stageFilter}
          onChange={(e) => onFilter(statusFilter, e.target.value, labelFilter)}
        />
        <input
          aria-label="标签筛选"
          data-testid="filter-label"
          placeholder="标签"
          value={labelFilter}
          onChange={(e) => onFilter(statusFilter, stageFilter, e.target.value)}
        />
        <button data-testid="view-board" onClick={() => setView('board')}>
          看板
        </button>
        <button data-testid="view-list" onClick={() => setView('list')}>
          列表
        </button>
        <button data-testid="toggle-create" onClick={() => setShowCreate((v) => !v)}>
          新建 Issue
        </button>
      </div>

      {showCreate && (
        <form onSubmit={onCreate} className="inline-form" data-testid="create-form">
          <input
            data-testid="create-title"
            placeholder="标题"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            required
          />
          <input
            data-testid="create-description"
            placeholder="描述"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
          <input
            data-testid="create-stage"
            type="number"
            min={0}
            placeholder="Stage"
            value={stage}
            onChange={(e) => setStage(e.target.value)}
          />
          <select
            aria-label="优先级"
            data-testid="create-priority"
            value={priority}
            onChange={(e) => setPriority(e.target.value)}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {PRIORITY_LABELS[p]}
              </option>
            ))}
          </select>
          <input
            data-testid="create-labels"
            placeholder="标签（逗号分隔）"
            value={labels}
            onChange={(e) => setLabels(e.target.value)}
          />
          <input
            data-testid="create-due"
            type="date"
            aria-label="截止日"
            value={dueDate}
            onChange={(e) => setDueDate(e.target.value)}
          />
          <button type="submit" data-testid="submit-create">
            创建
          </button>
        </form>
      )}

      {issues === null ? (
        <p>加载中…</p>
      ) : view === 'board' ? (
        <div className="board-columns">
          {STATUSES.map((s) => (
            <div key={s} className="board-column" data-testid={`column-${s}`}>
              <h3>
                {STATUS_LABELS[s]} <span className="count">{byStatus(visible, s).length}</span>
              </h3>
              {byStatus(visible, s).map((i) => (
                <IssueCard key={i.id} issue={i} projectId={id} onTransition={onTransition} onPriorityChange={onPriorityChange} />
              ))}
            </div>
          ))}
        </div>
      ) : (
        <div className="stage-groups">
          {Array.from(new Set(visible.map((i) => i.stage)))
            .sort((a, b) => a - b)
            .map((stageNum) => (
              <div key={stageNum} data-testid={`stage-group-${stageNum}`}>
                <h3>Stage {stageNum}</h3>
                {byStage(stageNum).map((i) => (
                  <IssueCard key={i.id} issue={i} projectId={id} onTransition={onTransition} onPriorityChange={onPriorityChange} />
                ))}
              </div>
            ))}
        </div>
      )}
    </section>
  )
}

function byStatus(issues: Issue[], status: string): Issue[] {
  return issues.filter((i) => i.status === status)
}
