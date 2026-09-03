import { useEffect, useState } from 'react'
import { apiFetch, ApiError } from '../api/client'

interface Project {
  id: string
  name: string
}

export function ProjectsPage() {
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    apiFetch<{ projects: Project[] }>('/projects')
      .then((res) => {
        if (!cancelled) setProjects(res.projects ?? [])
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : '加载失败')
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) return <p role="alert">{error}</p>
  if (projects === null) return <p>加载中…</p>
  if (projects.length === 0) return <p>还没有项目，后端就绪后可在此创建。</p>

  return (
    <ul className="project-list">
      {projects.map((p) => (
        <li key={p.id}>{p.name}</li>
      ))}
    </ul>
  )
}
