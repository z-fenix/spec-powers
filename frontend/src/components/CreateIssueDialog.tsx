import { useEffect, useState, type FormEvent } from 'react'
import { apiFetch, ApiError } from '../api/client'
import { createIssue } from '../api/issues'
import { PRIORITIES, PRIORITY_LABELS, parseLabels } from '../lib/status'
import { Modal } from './Modal'

interface Project {
  id: string
  name: string
  archived: boolean
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

export function CreateIssueDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated?: (projectId: string, issueId: string) => void
}) {
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [projectId, setProjectId] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [stage, setStage] = useState('')
  const [priority, setPriority] = useState('none')
  const [labels, setLabels] = useState('')
  const [dueDate, setDueDate] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    apiFetch<{ projects: Project[] }>('/projects')
      .then((res) => {
        const list = (res.projects ?? []).filter((p) => !p.archived)
        setProjects(list)
        setProjectId((cur) => cur || list[0]?.id || '')
      })
      .catch((err) => setError(errorMessage(err, '加载项目失败')))
  }, [open])

  if (!open) return null

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!projectId || !title.trim() || submitting) return
    setError('')
    setSubmitting(true)
    createIssue(projectId, {
      title,
      description,
      stage: stage ? Number(stage) : undefined,
      priority,
      labels: labels ? parseLabels(labels) : undefined,
      due_date: dueDate || undefined,
    })
      .then((issue) => {
        setTitle('')
        setDescription('')
        setStage('')
        setPriority('none')
        setLabels('')
        setDueDate('')
        onClose()
        onCreated?.(projectId, issue.id)
      })
      .catch((err) => setError(errorMessage(err, '创建失败')))
      .finally(() => setSubmitting(false))
  }

  return (
    <Modal title="新建任务" description="选择项目并创建一个新的任务。" onClose={onClose}>
      <form onSubmit={onSubmit} data-testid="global-create-form">
        <label>
          项目
          <select
            className="input"
            data-testid="global-create-project"
            value={projectId}
            onChange={(e) => setProjectId(e.target.value)}
            required
          >
            {(projects ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          标题
          <input
            className="input"
            data-testid="global-create-title"
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
            data-testid="global-create-description"
            placeholder="描述"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          Stage
          <input
            className="input"
            data-testid="global-create-stage"
            type="number"
            min={0}
            placeholder="Stage"
            value={stage}
            onChange={(e) => setStage(e.target.value)}
          />
        </label>
        <label>
          优先级
          <select
            className="input"
            data-testid="global-create-priority"
            value={priority}
            onChange={(e) => setPriority(e.target.value)}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {PRIORITY_LABELS[p]}
              </option>
            ))}
          </select>
        </label>
        <label>
          标签
          <input
            className="input"
            data-testid="global-create-labels"
            placeholder="标签（逗号分隔）"
            value={labels}
            onChange={(e) => setLabels(e.target.value)}
          />
        </label>
        <label>
          截止日
          <input
            className="input"
            data-testid="global-create-due"
            type="date"
            aria-label="截止日"
            value={dueDate}
            onChange={(e) => setDueDate(e.target.value)}
          />
        </label>
        {error && (
          <p role="alert" style={{ margin: 0, color: 'var(--destructive)', fontSize: 'var(--text-body)' }}>
            {error}
          </p>
        )}
        <div className="modal-actions">
          <button type="button" className="btn btn-outline" onClick={onClose}>
            取消
          </button>
          <button type="submit" className="btn btn-primary" data-testid="global-create-submit" disabled={submitting}>
            创建
          </button>
        </div>
      </form>
    </Modal>
  )
}
