import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch, ApiError } from '../api/client'

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

  return (
    <section>
      <h2>项目</h2>
      {error && (
        <p role="alert" data-testid="projects-error">
          {error}
        </p>
      )}

      <form onSubmit={onCreate} className="inline-form">
        <input
          data-testid="project-name"
          placeholder="项目名称"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <input
          data-testid="project-description"
          placeholder="描述（可选）"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <button type="submit" data-testid="create-project">
          创建项目
        </button>
      </form>

      {projects && projects.length === 0 && <p>还没有项目，先创建一个。</p>}

      <ul className="project-list">
        {(projects ?? []).map((p) => (
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
    </section>
  )
}
