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
import { STATUSES, STATUS_LABELS } from '../lib/status'

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
  onDragStart: (issueId: string) => void
  onDropOnCard: (dragId: string, targetIssueId: string) => void
}

function IssueCard({
  issue,
  projectId,
  onTransition,
  onDragStart,
  onDropOnCard,
}: IssueCardProps) {
  const [showArtifacts, setShowArtifacts] = useState(false)
  return (
    <div
      className="issue-card"
      data-testid={`card-${issue.id}`}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', issue.id)
        onDragStart(issue.id)
      }}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault()
        e.stopPropagation()
        const dragId = e.dataTransfer.getData('text/plain')
        if (dragId && dragId !== issue.id) onDropOnCard(dragId, issue.id)
      }}
    >
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
  const [searchQuery, setSearchQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [stage, setStage] = useState('')
  const [dragIssueId, setDragIssueId] = useState('')

  const load = useCallback(
    (filter?: { status?: string; stage?: number; query?: string }) => {
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
    const filter: { status?: string; stage?: number; query?: string } = {}
    if (status) filter.status = status
    if (stage) filter.stage = Number(stage)
    if (searchQuery) filter.query = searchQuery
    load(filter)
  }

  const onSearch = (e: FormEvent) => {
    e.preventDefault()
    const q = searchInput.trim()
    setSearchQuery(q)
    const filter: { status?: string; stage?: number; query?: string } = {}
    if (statusFilter) filter.status = statusFilter
    if (stageFilter) filter.stage = Number(stageFilter)
    if (q) filter.query = q
    load(filter)
  }

  const onTransition = (issueId: string, status: string) => {
    setError('')
    transitionIssue(id, issueId, status)
      .then(() => load())
      .catch((err) => setError(errorMessage(err, '状态流转失败')))
  }

  // Dropping a card in a column either flows its status (cross-column) or
  // reorders it within the column (same-column), persisting positions.
  const onDropInColumn = (dragId: string, targetStatus: string, targetIndex: number | null) => {
    const all = issues ?? []
    const dragged = all.find((i) => i.id === dragId)
    if (!dragged) return
    if (dragged.status !== targetStatus) {
      onTransition(dragId, targetStatus)
      return
    }
    const column = all.filter((i) => i.status === targetStatus)
    const from = column.findIndex((i) => i.id === dragId)
    const to =
      targetIndex === null
        ? column.length - 1
        : Math.max(0, Math.min(targetIndex, column.length - 1))
    if (from < 0 || from === to) return
    const reordered = column.slice()
    reordered.splice(from, 1)
    reordered.splice(to, 0, dragged)
    const updates = reordered
      .map((issue, idx) => ({ issue, idx }))
      .filter(({ issue, idx }) => issue.position !== idx)
      .map(({ issue, idx }) => updateIssue(id, issue.id, { position: idx }))
    const repositioned = reordered.map((issue, idx) =>
      issue.position === idx ? issue : { ...issue, position: idx },
    )
    setIssues([...all.filter((i) => i.status !== targetStatus), ...repositioned])
    Promise.all(updates)
      .then(() => load())
      .catch((err) => setError(errorMessage(err, '排序失败')))
  }

  const onDropOnCard = (dragId: string, targetIssueId: string) => {
    const target = (issues ?? []).find((i) => i.id === targetIssueId)
    if (!target) return
    const column = (issues ?? []).filter((i) => i.status === target.status)
    onDropInColumn(dragId, target.status, column.findIndex((i) => i.id === targetIssueId))
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
        <form onSubmit={onSearch} className="inline-form" data-testid="search-form">
          <input
            aria-label="搜索 Issue"
            data-testid="search-issues"
            type="search"
            placeholder="搜索标题/描述/评论"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
          <button type="submit" data-testid="search-submit">
            搜索
          </button>
        </form>
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
            <div
              key={s}
              className="board-column"
              data-testid={`column-${s}`}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                const dragId = e.dataTransfer.getData('text/plain') || dragIssueId
                if (dragId) onDropInColumn(dragId, s, null)
              }}
            >
              <h3>
                {STATUS_LABELS[s]} <span className="count">{byStatus(visible, s).length}</span>
              </h3>
              {byStatus(visible, s).map((i) => (
                <IssueCard
                  key={i.id}
                  issue={i}
                  projectId={id}
                  onTransition={onTransition}
                  onDragStart={setDragIssueId}
                  onDropOnCard={onDropOnCard}
                />
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
                  <IssueCard
                    key={i.id}
                    issue={i}
                    projectId={id}
                    onTransition={onTransition}
                    onDragStart={setDragIssueId}
                    onDropOnCard={onDropOnCard}
                  />
                ))}
              </div>
            ))}
        </div>
      )}
    </section>
  )
}

function byStatus(issues: Issue[], status: string): Issue[] {
  return issues
    .filter((i) => i.status === status)
    .sort((a, b) => a.position - b.position)
}
