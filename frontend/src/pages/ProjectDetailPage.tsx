import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { apiFetch, ApiError } from '../api/client'
import { Modal } from '../components/Modal'
import { ProjectUsagePanel } from '../components/Usage'
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
  const [resourceError, setResourceError] = useState('')

  const [resourceType, setResourceType] = useState('github_repo')
  const [label, setLabel] = useState('')
  const [pointer, setPointer] = useState('')
  const [showResource, setShowResource] = useState(false)

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
    setResourceError('')
    apiFetch<{ resource: Resource }>(`/projects/${id}/resources`, {
      method: 'POST',
      body: { type: resourceType, label, pointer },
    })
      .then(() => {
        setLabel('')
        setPointer('')
        setShowResource(false)
        return apiFetch<{ resources: Resource[] }>(`/projects/${id}/resources`)
      })
      .then((r) => setResources(r.resources ?? []))
      .catch((err) => setResourceError(errorMessage(err, '添加资源失败')))
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
    <div className="page">
      <div className="page-header">
        <nav className="breadcrumb">
          <Link to="/">项目</Link>
          <span className="crumb-sep">›</span>
          <span className="crumb-leaf">{project.name}</span>
        </nav>
        {project.archived && <span className="badge">已归档</span>}
        <div className="page-header-actions">
          <button
            type="button"
            className="btn btn-outline btn-sm"
            data-testid="add-resource"
            onClick={() => {
              setResourceError('')
              setShowResource(true)
            }}
          >
            添加资源
          </button>
          <Link className="btn btn-primary btn-sm" data-testid="board-link" to={`/projects/${id}/board`}>
            Issue 看板
          </Link>
        <button data-testid="toggle-archive" onClick={onToggleArchive}>
          {project.archived ? '恢复' : '归档'}
        </button>
      </div>
      </div>
      {error && (
        <p role="alert" data-testid="detail-error" style={{ margin: 0, padding: '8px 16px', color: 'var(--destructive)' }}>
          {error}
        </p>
      )}
      <div className="page-body">
        <div className="page-gutter">
          {project.description && <p className="detail-desc" style={{ marginTop: 0 }}>{project.description}</p>}
          {notice && <p className="notice">{notice}</p>}

          <div className="detail-section">
            <h3 className="section-title">资源绑定</h3>
            {resources.length === 0 && <p className="empty-state" style={{ padding: '16px 0' }}>还没有绑定资源。</p>}
            <ul className="project-list">
              {resources.map((r) => (
                <li key={r.id} className="collection-row">
                  <span className="badge">{r.type === 'github_repo' ? 'GitHub 仓库' : '本地目录'}</span>
                  <div className="collection-row-main">
                    <strong>{r.label}</strong>
                    <code>{r.pointer}</code>
                  </div>
                  <button
                    className="btn btn-ghost btn-sm"
                    data-testid={`remove-${r.id}`}
                    onClick={() => onRemoveResource(r.id)}
                  >
                    移除
                  </button>
                </li>
              ))}
            </ul>
          </div>

          <ProjectUsagePanel projectId={id} />

          <div className="detail-section">
            <h3 className="section-title">项目上下文</h3>
            <textarea
              data-testid="context-input"
              rows={6}
              style={{ width: '100%' }}
              value={context}
              onChange={(e) => setContext(e.target.value)}
            />
            <div style={{ marginTop: 8 }}>
              <button className="btn btn-primary btn-sm" data-testid="save-context" onClick={onSaveContext}>
                保存上下文
              </button>
            </div>
          </div>
        </div>
      </div>
      {showResource && (
        <Modal
          title="添加资源"
          description="把代码仓库或本地目录绑定到这个项目。"
          onClose={() => setShowResource(false)}
        >
          <form onSubmit={onAddResource}>
            <label>
              类型
              <select
                data-testid="resource-type"
                value={resourceType}
                onChange={(e) => setResourceType(e.target.value)}
              >
                <option value="github_repo">GitHub 仓库</option>
                <option value="local_directory">本地目录</option>
              </select>
            </label>
            <label>
              标签
              <input
                className="input"
                data-testid="resource-label"
                placeholder="标签"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                required
                autoFocus
              />
            </label>
            <label>
              指向
              <input
                className="input"
                data-testid="resource-pointer"
                placeholder={resourceType === 'github_repo' ? 'owner/repo' : '绝对路径'}
                value={pointer}
                onChange={(e) => setPointer(e.target.value)}
                required
              />
            </label>
            {resourceError && (
              <p role="alert" style={{ margin: 0, color: 'var(--destructive)', fontSize: 'var(--text-body)' }}>
                {resourceError}
              </p>
            )}
            <div className="modal-actions">
              <button type="button" className="btn btn-outline" onClick={() => setShowResource(false)}>
                取消
              </button>
              <button type="submit" className="btn btn-primary" data-testid="confirm-add-resource">
                添加
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
