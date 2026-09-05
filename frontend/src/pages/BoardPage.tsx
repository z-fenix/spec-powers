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
import { listStatuses, upsertStatus, deleteStatus } from '../api/statuses'
import { DEFAULT_DIRECTORY, STATUS_CATEGORIES, CATEGORY_LABELS, statusLabel, type StatusEntry } from '../lib/status'
import { STATUSES, STATUS_LABELS } from '../lib/status'
import {
  decodeMultiSelect,
  listProjectIssueProperties,
  listPropertyDefinitions,
  type IssuePropertyValue,
  type PropertyDefinition,
} from '../api/properties'
import { StatusIcon } from '../components/StatusIcon'
import { Modal } from '../components/Modal'

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
  directory: StatusEntry[]
  onTransition: (issueId: string, status: string) => void
  onDragStart: (issueId: string) => void
  onDropOnCard: (dragId: string, targetIssueId: string) => void
}

function IssueCard({
  issue,
  projectId,
  directory,
  onTransition,
  onDragStart,
  onDropOnCard,
}: IssueCardProps) {
  const [showArtifacts, setShowArtifacts] = useState(false)
  const category = directory.find((s) => s.name === issue.status)?.category
  return (
    <div
      className="board-card"
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
      <div className="board-card-top">
        <StatusIcon status={issue.status} category={category} />
        {issue.stage > 0 && <span className="chip">S{issue.stage}</span>}
      </div>
      <p className="board-card-title">
        <Link to={`/projects/${projectId}/issues/${issue.id}`}>{issue.title}</Link>
      </p>
      <div className="board-card-meta">
        <select
          aria-label="状态"
          data-testid={`status-${issue.id}`}
          value={issue.status}
          onChange={(e) => onTransition(issue.id, e.target.value)}
        >
          {directory.map((s) => (
            <option key={s.name} value={s.name}>
              {statusLabel(s.name)}
            </option>
          ))}
        </select>
      </div>
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
  const [directory, setDirectory] = useState<StatusEntry[]>(DEFAULT_DIRECTORY)
  const [error, setError] = useState('')
  const [view, setView] = useState<'board' | 'list'>('board')
  const [statusFilter, setStatusFilter] = useState('')
  const [stageFilter, setStageFilter] = useState('')
  const [propertyDefs, setPropertyDefs] = useState<PropertyDefinition[]>([])
  const [propertyValues, setPropertyValues] = useState<IssuePropertyValue[]>([])
  const [propertyFilter, setPropertyFilter] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [showDirectory, setShowDirectory] = useState(false)
  const [newStatusName, setNewStatusName] = useState('')
  const [newStatusCategory, setNewStatusCategory] = useState<string>(STATUS_CATEGORIES[1])
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
    listPropertyDefinitions(id)
      .then((defs) => {
        setPropertyDefs(defs)
        if (defs.some((d) => d.type === 'select')) {
          return listProjectIssueProperties(id).then(setPropertyValues)
        }
        return undefined
      })
      .catch(() => {
        setPropertyDefs([])
        setPropertyValues([])
      })
  }, [load, id])

  // Status directory drives the kanban columns; fall back to the built-in
  // seven when the workspace directory cannot be loaded.
  useEffect(() => {
    let cancelled = false
    listStatuses(id)
      .then((list) => {
        if (cancelled) return
        setDirectory(list.length > 0 ? list : DEFAULT_DIRECTORY)
      })
      .catch(() => {
        if (!cancelled) setDirectory(DEFAULT_DIRECTORY)
      })
    return () => {
      cancelled = true
    }
  }, [id])

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

  const allIssues = issues ?? []
  const selectDefs = propertyDefs.filter((d) => d.type === 'select')
  const visible = (() => {
    if (!propertyFilter) return allIssues
    const idx = propertyFilter.indexOf('|')
    const propId = propertyFilter.slice(0, idx)
    const option = propertyFilter.slice(idx + 1)
    const byIssue = new Map(propertyValues.map((v) => [v.issue_id, v]))
    return allIssues.filter((i) => {
      const v = byIssue.get(i.id)
      if (!v || v.property_id !== propId) return false
      if (v.value.startsWith('[')) return decodeMultiSelect(v.value).includes(option)
      return v.value === option
    })
  })()
  const byStage = (stage: number) => visible.filter((i) => i.stage === stage)

  return (
    <div className="page" data-testid="board">
      <div className="page-header">
        <h1 className="page-title">Issue 看板</h1>
        {error && !showCreate && (
            <p role="alert" data-testid="board-error" style={{ margin: 0, color: 'var(--destructive)', fontSize: 'var(--text-caption)' }}>
              {error}
            </p>
        )}
      </div>
      <div className="board-toolbar">
        <div className="toolbar-group">
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
            className="input"
            style={{ width: 96 }}
          aria-label="Stage 筛选"
          data-testid="filter-stage"
          type="number"
          min={0}
          placeholder="Stage"
          value={stageFilter}
          onChange={(e) => onFilter(statusFilter, e.target.value)}
        />
        {selectDefs.length > 0 && (
          <select
              className="input"
              aria-label="属性筛选"
            data-testid="filter-property"
            value={propertyFilter}
            onChange={(e) => setPropertyFilter(e.target.value)}
          >
            <option value="">全部属性</option>
            {selectDefs.map((d) => (
              <optgroup key={d.id} label={d.name}>
                {d.options.map((o) => (
                  <option key={o} value={`${d.id}|${o}`}>
                    {`${d.name}: ${o}`}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
        )}
        <button data-testid="view-board" onClick={() => setView('board')}>
          看板
        </button>
        <button data-testid="view-list" onClick={() => setView('list')}>
          列表
        </button>
        <button data-testid="toggle-create" onClick={() => setShowCreate((v) => !v)}>
          新建 Issue
        </button>
            <form onSubmit={onSearch}  className="inline-form" data-testid="board-search">
                <input
                    className="input"
                    aria-label="搜索 Issue"
                    data-testid="search-issues"
                    type="search"
                    placeholder="搜索标题/描述/评论"
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                />
                <button type="submit" className="btn btn-outline btn-sm" data-testid="search-submit">
                    搜索
                </button>
            </form>
        <div className="toolbar-group">
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
            className="input"
            style={{ width: 96 }}
            aria-label="Stage 筛选"
            data-testid="filter-stage"
            type="number"
            min={0}
            placeholder="Stage"
            value={stageFilter}
            onChange={(e) => onFilter(statusFilter, e.target.value)}
          />
            <button
              className={view === 'board' ? 'btn btn-outline btn-sm' : 'btn btn-ghost btn-sm'}
              data-testid="view-board"
              onClick={() => setView('board')}
            >
              看板
            </button>
            <button
              className={view === 'list' ? 'btn btn-outline btn-sm' : 'btn btn-ghost btn-sm'}
              data-testid="view-list"
              onClick={() => setView('list')}
            >
              列表
            </button>
            <button
              className="btn btn-outline btn-sm"
              data-testid="toggle-statuses"
              onClick={() => { setError(''); setShowDirectory(true) }}
            >
              状态目录
            </button>
            <button className="btn btn-primary btn-sm" data-testid="toggle-create" onClick={() => { setError(''); setShowCreate(true) }}>
              新建 Issue
            </button>
          </div>
        </div>
      </div>

      <div className="page-body">
        {issues === null ? (
          <p className="page-gutter">加载中…</p>
        ) : view === 'board' ? (
          <div className="board">
            {[...directory]
              .sort((a, b) => a.position - b.position)
              .map((s) => (
              <div
                key={s.name}
                className="board-col"
                data-status={s.name}
                data-testid={`column-${s.name}`}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => {
                  e.preventDefault()
                  const dragId = e.dataTransfer.getData('text/plain') || dragIssueId
                  if (dragId) onDropInColumn(dragId, s.name, null)
                }}
              >
                <div className="board-col-header">
                  <StatusIcon status={s.name} category={s.category} />
                  {statusLabel(s.name)}
                  <span className="board-col-count">{byStatus(visible, s.name).length}</span>
                </div>
                <div className="board-col-cards">
                  {byStatus(visible, s.name).map((i) => (
                    <IssueCard
                      key={i.id}
                      issue={i}
                      projectId={id}
                      directory={directory}
                      onTransition={onTransition}
                      onDragStart={setDragIssueId}
                      onDropOnCard={onDropOnCard}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="page-gutter">
            {Array.from(new Set(visible.map((i) => i.stage)))
              .sort((a, b) => a - b)
              .map((stageNum) => (
                <div key={stageNum} className="stage-group" data-testid={`stage-group-${stageNum}`}>
                  <h3 className="stage-group-title">Stage {stageNum}</h3>
                  {byStage(stageNum).map((i) => (
                    <IssueCard
                      key={i.id}
                      issue={i}
                      projectId={id}
                      directory={directory}
                      onTransition={onTransition}
                      onDragStart={setDragIssueId}
                      onDropOnCard={onDropOnCard}
                    />
                  ))}
                </div>
              ))}
          </div>
        )}
      </div>

      {showCreate && (
        <Modal title="新建 Issue" description="在看板上创建一个新的 Issue。" onClose={() => setShowCreate(false)}>
          <form onSubmit={onCreate} data-testid="create-form">
            <label>
              标题
              <input
                className="input"
                data-testid="create-title"
                placeholder="标题"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                required
                autoFocus
              />
            </label>
            <label>
              描述
              <input
                className="input"
                data-testid="create-description"
                placeholder="描述"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </label>
            <label>
              Stage
              <input
                className="input"
                data-testid="create-stage"
                type="number"
                min={0}
                placeholder="Stage"
                value={stage}
                onChange={(e) => setStage(e.target.value)}
              />
            </label>
            {error && (
              <p role="alert" style={{ margin: 0, color: 'var(--destructive)', fontSize: 'var(--text-body)' }}>
                {error}
              </p>
            )}
            <div className="modal-actions">
              <button type="button" className="btn btn-outline" onClick={() => setShowCreate(false)}>
                取消
              </button>
              <button type="submit" className="btn btn-primary" data-testid="submit-create">
                创建
              </button>
            </div>
          </form>
        </Modal>
      )}

      {showDirectory && (
        <Modal
          title="状态目录"
          description="工作区的状态目录定义看板列与状态流转类别。"
          onClose={() => setShowDirectory(false)}
        >
          <ul data-testid="status-directory">
            {directory.map((s) => (
              <li key={s.name} data-testid={`dir-row-${s.name}`}>
                {statusLabel(s.name)} · {CATEGORY_LABELS[s.category] ?? s.category}
                <button
                  data-testid={`dir-delete-${s.name}`}
                  onClick={() => {
                    setError('')
                    deleteStatus(id, s.name)
                      .then((list) => setDirectory(list.length > 0 ? list : DEFAULT_DIRECTORY))
                      .catch((err) => setError(errorMessage(err, '删除状态失败')))
                  }}
                >
                  删除
                </button>
              </li>
            ))}
          </ul>
          <form
            data-testid="dir-form"
            onSubmit={(e) => {
              e.preventDefault()
              setError('')
              upsertStatus(id, newStatusName, newStatusCategory)
                .then((list) => {
                  setDirectory(list)
                  setNewStatusName('')
                  setNewStatusCategory(STATUS_CATEGORIES[1])
                })
                .catch((err) => setError(errorMessage(err, '保存状态失败')))
            }}
          >
            <label>
              名称
              <input
                className="input"
                data-testid="dir-name"
                value={newStatusName}
                onChange={(e) => setNewStatusName(e.target.value)}
                required
              />
            </label>
            <label>
              类别
              <select
                data-testid="dir-category"
                value={newStatusCategory}
                onChange={(e) => setNewStatusCategory(e.target.value)}
              >
                {STATUS_CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {CATEGORY_LABELS[c]}
                  </option>
                ))}
              </select>
            </label>
            <button type="submit" className="btn btn-primary" data-testid="dir-submit">
              添加状态
            </button>
          </form>
        </Modal>
      )}
    </div>
  )
}

function byStatus(issues: Issue[], status: string): Issue[] {
  return issues
    .filter((i) => i.status === status)
    .sort((a, b) => a.position - b.position)
}
