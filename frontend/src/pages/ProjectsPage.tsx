import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch, ApiError } from '../api/client'
import { Modal } from '../components/Modal'

export interface Project {
  id: string
  workspace_id: string
  name: string
  description: string
  archived: boolean
  created_by: string
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

export function ProjectsPage() {
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  const load = useCallback(() => {
    apiFetch<{ projects: Project[] }>('/projects')
      .then((res) => setProjects(res.projects ?? []))
      .catch((err) => setError(errorMessage(err, '加载失败')))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const onCreate = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    apiFetch<{ project: Project }>('/projects', { method: 'POST', body: { name, description } })
      .then(() => {
        setName('')
        setDescription('')
        setShowCreate(false)
        load()
      })
      .catch((err) => setError(errorMessage(err, '创建失败')))
  }

  const onArchive = (p: Project, archived: boolean) => {
    setError('')
    apiFetch<{ project: Project }>(`/projects/${p.id}/archive`, { method: 'POST', body: { archived } })
      .then(load)
      .catch((err) => setError(errorMessage(err, '操作失败')))
  }

  if (projects === null && !error) return <p>加载中…</p>

  // Archived projects sink to the end so active ones stay on top.
  const sorted = [...(projects ?? [])].sort(
    (a, b) => Number(a.archived) - Number(b.archived),
  )

  return (
    <div className="page" data-testid="projects-page">
      <div className="page-header">
        <span className="project-icon">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden>
            <path
              d="M1.75 4.5c0-.69.56-1.25 1.25-1.25h3l1.5 1.75h6c.69 0 1.25.56 1.25 1.25v5.25c0 .69-.56 1.25-1.25 1.25H3c-.69 0-1.25-.56-1.25-1.25V4.5Z"
              stroke="currentColor"
              strokeWidth="1.3"
              strokeLinejoin="round"
            />
          </svg>
        </span>
        <h1 className="page-title">
          项目
          {projects && <span className="page-count">{projects.length}</span>}
        </h1>
        <div className="page-header-actions">
          <button
            type="button"
            className="btn btn-primary btn-sm"
            data-testid="create-project"
            onClick={() => {
              setError('')
              setShowCreate(true)
            }}
          >
            创建项目
          </button>
        </div>
      </div>
      <div className="page-body">
        <div className="page-gutter">
          {error && !showCreate && (
            <p role="alert" data-testid="projects-error">
              {error}
            </p>
          )}

          {projects && projects.length === 0 && <p>还没有项目，先创建一个。</p>}

          <ul className="project-list">
            {sorted.map((p) => (
              <li key={p.id} className={p.archived ? 'project-item archived' : 'project-item'}>
                <div className="project-item-main">
                  <Link to={`/projects/${p.id}`}>{p.name}</Link>
                  {p.archived && <span className="badge">已归档</span>}
                  {p.description && <p className="project-desc">{p.description}</p>}
                </div>
                <button data-testid={`archive-${p.id}`} onClick={() => onArchive(p, !p.archived)}>
                  {p.archived ? '恢复' : '归档'}
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
      {showCreate && (
        <Modal
          title="创建项目"
          description="为工作区新建一个项目。"
          onClose={() => setShowCreate(false)}
        >
          <form onSubmit={onCreate}>
            <label>
              名称
              <input
                className="input"
                data-testid="project-name"
                placeholder="项目名称"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                autoFocus
              />
            </label>
            <label>
              描述
              <input
                className="input"
                data-testid="project-description"
                placeholder="描述（可选）"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
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
              <button type="submit" className="btn btn-primary" data-testid="confirm-create-project">
                创建
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
