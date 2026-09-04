import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  createIssue,
  listIssues,
  transitionIssue,
  type Issue,
} from '../api/issues'
import { ApiError } from '../api/client'
import { STATUSES, STATUS_LABELS } from '../lib/status'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

interface IssueCardProps {
  issue: Issue
  projectId: string
  onTransition: (issueId: string, status: string) => void
}

function IssueCard({ issue, projectId, onTransition }: IssueCardProps) {
  return (
    <div className="issue-card" data-testid={`card-${issue.id}`}>
      <Link to={`/projects/${projectId}/issues/${issue.id}`}>{issue.title}</Link>
      {issue.stage > 0 && <span className="badge">S{issue.stage}</span>}
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
    </div>
  )
}

export function BoardPage() {
  const { id = '' } = useParams()
  const [issues, setIssues] = useState<Issue[] | null>(null)
  const [error, setError] = useState('')
  const [view, setView] = useState<'board' | 'list'>('board')
  const [statusFilter, setStatusFilter] = useState('')
  const [stageFilter, setStageFilter] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [stage, setStage] = useState('')

  const load = useCallback(
    (filter?: { status?: string; stage?: number }) => {
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

  const onFilter = (status: string, stage: string) => {
    setStatusFilter(status)
    setStageFilter(stage)
    const filter: { status?: string; stage?: number } = {}
    if (status) filter.status = status
    if (stage) filter.stage = Number(stage)
    load(filter)
  }

  const onTransition = (issueId: string, status: string) => {
    setError('')
    transitionIssue(id, issueId, status)
      .then(() => load())
      .catch((err) => setError(errorMessage(err, '状态流转失败')))
  }

  const onCreate = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    createIssue(id, {
      title,
      description,
      stage: stage ? Number(stage) : undefined,
    })
      .then(() => {
        setTitle('')
        setDescription('')
        setStage('')
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
          onChange={(e) => onFilter(e.target.value, stageFilter)}
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
          onChange={(e) => onFilter(statusFilter, e.target.value)}
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
                <IssueCard key={i.id} issue={i} projectId={id} onTransition={onTransition} />
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
                  <IssueCard key={i.id} issue={i} projectId={id} onTransition={onTransition} />
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
