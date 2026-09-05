import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { apiFetch, ApiError } from '../api/client'
import type { Project } from './ProjectsPage'

interface Resource {
  id: string
  type: string
  label: string
  pointer: string
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

export function ProjectDetailPage() {
  const { id = '' } = useParams()
  const [project, setProject] = useState<Project | null>(null)
  const [resources, setResources] = useState<Resource[] | null>(null)
  const [context, setContext] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const [resourceType, setResourceType] = useState('github_repo')
  const [label, setLabel] = useState('')
  const [pointer, setPointer] = useState('')

  const load = useCallback(() => {
    Promise.all([
      apiFetch<{ project: Project }>(`/projects/${id}`),
      apiFetch<{ resources: Resource[] }>(`/projects/${id}/resources`),
      apiFetch<{ context: { content: string } }>(`/projects/${id}/context`),
    ])
      .then(([p, r, c]) => {
        setProject(p.project)
        setResources(r.resources ?? [])
        setContext(c.context?.content ?? '')
      })
      .catch((err) => setError(errorMessage(err, '加载失败')))
  }, [id])

  useEffect(() => {
    load()
  }, [load])

  const flash = (msg: string) => {
    setNotice(msg)
    setTimeout(() => setNotice(''), 2000)
  }

  const onAddResource = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    apiFetch<{ resource: Resource }>(`/projects/${id}/resources`, {
      method: 'POST',
      body: { type: resourceType, label, pointer },
    })
      .then(() => {
        setLabel('')
        setPointer('')
        return apiFetch<{ resources: Resource[] }>(`/projects/${id}/resources`)
      })
      .then((r) => setResources(r.resources ?? []))
      .catch((err) => setError(errorMessage(err, '添加资源失败')))
  }

  const onRemoveResource = (resourceID: string) => {
    setError('')
    apiFetch<void>(`/projects/${id}/resources/${resourceID}`, { method: 'DELETE' })
      .then(() => apiFetch<{ resources: Resource[] }>(`/projects/${id}/resources`))
      .then((r) => setResources(r.resources ?? []))
      .catch((err) => setError(errorMessage(err, '移除资源失败')))
  }

  const onSaveContext = () => {
    setError('')
    apiFetch<{ context: { content: string } }>(`/projects/${id}/context`, {
      method: 'PUT',
      body: { content: context },
    })
      .then(() => flash('已保存'))
      .catch((err) => setError(errorMessage(err, '保存上下文失败')))
  }

  const onToggleArchive = () => {
    if (!project) return
    setError('')
    apiFetch<{ project: Project }>(`/projects/${id}/archive`, {
      method: 'POST',
      body: { archived: !project.archived },
    })
      .then(() => {
        flash(project.archived ? '已恢复' : '已归档')
        load()
      })
      .catch((err) => setError(errorMessage(err, '操作失败')))
  }

  if (error && !project) return <p role="alert">{error}</p>
  if (!project || resources === null) return <p>加载中…</p>

  return (
    <section>
      <h2>
        {project.name}
        {project.archived && <span className="badge">已归档</span>}
      </h2>
      {project.description && <p className="project-desc">{project.description}</p>}
      <p>
        <Link data-testid="board-link" to={`/projects/${id}/board`}>
          Issue 看板
        </Link>
        <button data-testid="toggle-archive" onClick={onToggleArchive}>
          {project.archived ? '恢复' : '归档'}
        </button>
      </p>
      {error && (
        <p role="alert" data-testid="detail-error">
          {error}
        </p>
      )}
      {notice && <p className="notice">{notice}</p>}

      <h3>资源绑定</h3>
      {resources.length === 0 && <p>还没有绑定资源。</p>}
      <ul className="project-list">
        {resources.map((r) => (
          <li key={r.id} className="project-item">
            <div className="project-item-main">
              <span className="badge">{r.type === 'github_repo' ? 'GitHub 仓库' : '本地目录'}</span>{' '}
              <strong>{r.label}</strong> <code>{r.pointer}</code>
            </div>
            <button data-testid={`remove-${r.id}`} onClick={() => onRemoveResource(r.id)}>
              移除
            </button>
          </li>
        ))}
      </ul>

      <form onSubmit={onAddResource} className="inline-form">
        <select
          data-testid="resource-type"
          value={resourceType}
          onChange={(e) => setResourceType(e.target.value)}
        >
          <option value="github_repo">GitHub 仓库</option>
          <option value="local_directory">本地目录</option>
        </select>
        <input
          data-testid="resource-label"
          placeholder="标签"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          required
        />
        <input
          data-testid="resource-pointer"
          placeholder={resourceType === 'github_repo' ? 'owner/repo' : '绝对路径'}
          value={pointer}
          onChange={(e) => setPointer(e.target.value)}
          required
        />
        <button type="submit" data-testid="add-resource">
          添加资源
        </button>
      </form>

      <h3>项目上下文</h3>
      <textarea
        data-testid="context-input"
        rows={6}
        style={{ width: '100%' }}
        value={context}
        onChange={(e) => setContext(e.target.value)}
      />
      <button data-testid="save-context" onClick={onSaveContext}>
        保存上下文
      </button>
    </section>
  )
}
