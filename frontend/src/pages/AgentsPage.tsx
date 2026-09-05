import { useEffect, useState } from 'react'
import { listAgents, type Agent } from '../api/agents'

export function AgentsPage() {
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    listAgents()
      .then((list) => {
        if (!cancelled) setAgents(list)
      })
      .catch(() => {
        if (!cancelled) {
          setError('加载失败')
          setAgents([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) return <p role="alert">{error}</p>
  if (agents === null) return <p>加载中…</p>

  return (
    <section data-testid="agents-page">
      <h2>Agents</h2>
      {agents.length === 0 && <p>暂无 Agent。</p>}
      <ul className="agent-list">
        {agents.map((a) => (
          <li key={a.id} data-testid={`agent-row-${a.id}`}>
            <span className="agent-name">{a.name}</span>
            <span className="badge">{a.runtime}</span>
            {a.description && <span className="agent-description">{a.description}</span>}
            {a.skills.length > 0 && (
              <span className="agent-skills">
                {a.skills.map((s) => (
                  <span key={s} className="badge">
                    {s}
                  </span>
                ))}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
