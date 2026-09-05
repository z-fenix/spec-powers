import { useCallback, useEffect, useState } from 'react'
import {
  createAgent,
  deleteAgent,
  listAgents,
  registerAgent,
  updateAgent,
  type Agent,
} from '../api/agents'
import { ApiError } from '../api/client'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

function parseSkills(text: string): string[] {
  return text
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

const RUNTIME_LABELS: Record<string, string> = {
  server: '服务端',
  local: '本地',
}

function AgentEditor({
  agent,
  name,
  description,
  skills,
  onName,
  onDescription,
  onSkills,
  onSave,
  onCancel,
}: {
  agent?: Agent
  name: string
  description: string
  skills: string
  onName: (v: string) => void
  onDescription: (v: string) => void
  onSkills: (v: string) => void
  onSave: () => void
  onCancel?: () => void
}) {
  return (
    <div className={agent ? 'edit-form' : 'inline-form'} data-testid={agent ? `agent-edit-${agent.id}` : 'agent-create-form'}>
      {agent ? (
        <input
          data-testid={`edit-agent-name-${agent.id}`}
          placeholder="名称"
          value={name}
          onChange={(e) => onName(e.target.value)}
        />
      ) : (
        <input
          data-testid="agent-name"
          placeholder="名称"
          value={name}
          onChange={(e) => onName(e.target.value)}
        />
      )}
      <input
        data-testid={agent ? `edit-agent-description-${agent.id}` : 'agent-description'}
        placeholder="描述（可选）"
        value={description}
        onChange={(e) => onDescription(e.target.value)}
      />
      <input
        data-testid={agent ? `edit-agent-skills-${agent.id}` : 'agent-skills'}
        placeholder="技能（逗号分隔，可选）"
        value={skills}
        onChange={(e) => onSkills(e.target.value)}
      />
      <button data-testid={agent ? `save-agent-${agent.id}` : 'create-agent'} onClick={onSave}>
        {agent ? '保存' : '创建 Agent'}
      </button>
      {agent && (
        <button data-testid={`cancel-agent-edit-${agent.id}`} onClick={onCancel}>
          取消
        </button>
      )}
    </div>
  )
}

export function AgentsPage() {
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [skills, setSkills] = useState('')
  const [runtime, setRuntime] = useState<'server' | 'local'>('server')
  const [registeredToken, setRegisteredToken] = useState('')

  const [editingId, setEditingId] = useState('')
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editSkills, setEditSkills] = useState('')

  const flash = (msg: string) => {
    setNotice(msg)
    setTimeout(() => setNotice(''), 4000)
  }

  const load = useCallback(() => {
    listAgents()
      .then(setAgents)
      .catch((err) => setError(errorMessage(err, '加载失败')))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const onCreate = () => {
    if (!name.trim()) {
      setError('请输入 Agent 名称。')
      return
    }
    setError('')
    setRegisteredToken('')
    const input = { name, description, skills: parseSkills(skills) }
    const call = runtime === 'local' ? registerAgent(input) : createAgent(input)
    call
      .then((res) => {
        if ('token' in res) {
          setRegisteredToken(res.token)
          flash(`本地 Agent ${res.agent.name} 注册成功，运行时凭证请立即保存。`)
        } else {
          flash(`Agent ${res.name} 创建成功。`)
        }
        setName('')
        setDescription('')
        setSkills('')
        load()
      })
      .catch((err) => setError(errorMessage(err, '创建失败')))
  }

  const startEdit = (a: Agent) => {
    setEditingId(a.id)
    setEditName(a.name)
    setEditDescription(a.description)
    setEditSkills(a.skills.join(', '))
  }

  const onSaveEdit = () => {
    if (!editingId) return
    setError('')
    updateAgent(editingId, {
      name: editName,
      description: editDescription,
      skills: parseSkills(editSkills),
    })
      .then(() => {
        setEditingId('')
        load()
      })
      .catch((err) => setError(errorMessage(err, '保存失败')))
  }

  const onDelete = (a: Agent) => {
    setError('')
    deleteAgent(a.id)
      .then(load)
      .catch((err) => setError(errorMessage(err, '删除失败')))
  }

  if (agents === null && !error) return <p>加载中…</p>

  return (
    <div className="page" data-testid="agents-page">
      <div className="page-header">
        <span className="project-icon"><svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden><rect x="2.75" y="4.75" width="10.5" height="8" rx="1.5" stroke="currentColor" strokeWidth="1.3"/><circle cx="5.75" cy="8.25" r="0.9" fill="currentColor"/><circle cx="10.25" cy="8.25" r="0.9" fill="currentColor"/><path d="M8 2.25V4.5M5.5 11.5h5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/></svg></span>
        <h1 className="page-title">
          Agents
          {agents && agents.length > 0 && (
            <span className="page-count">{agents.length}</span>
          )}
        </h1>
      </div>
      <div className="page-body">
      {error && (
        <p role="alert" data-testid="agents-error">
          {error}
        </p>
      )}
      {notice && (
        <p data-testid="agents-notice">{notice}</p>
      )}
      {registeredToken && (
        <p data-testid="register-token">
          <code>{registeredToken}</code>
        </p>
      )}

      <h3>新建 Agent</h3>
      <label>
        运行时{' '}
        <select
          data-testid="agent-runtime"
          value={runtime}
          onChange={(e) => setRuntime(e.target.value === 'local' ? 'local' : 'server')}
        >
          <option value="server">服务端</option>
          <option value="local">本地</option>
        </select>
      </label>
      <AgentEditor
        name={name}
        description={description}
        skills={skills}
        onName={setName}
        onDescription={setDescription}
        onSkills={setSkills}
        onSave={onCreate}
      />

      {agents && agents.length === 0 && <div className="page-empty" data-testid="page-empty">
          <span className="page-empty-icon"><svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden><rect x="2.75" y="4.75" width="10.5" height="8" rx="1.5" stroke="currentColor" strokeWidth="1.3"/><circle cx="5.75" cy="8.25" r="0.9" fill="currentColor"/><circle cx="10.25" cy="8.25" r="0.9" fill="currentColor"/><path d="M8 2.25V4.5M5.5 11.5h5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/></svg></span>
          <p className="page-empty-title">还没有 Agent</p>
          <p className="page-empty-desc">从右上角创建第一个智能体。</p>
        </div>}

      <ul className="project-list" data-testid="agent-list">
        {(agents ?? []).map((a) => (
          <li key={a.id} className="project-item" data-testid={`agent-${a.id}`}>
            <div className="project-item-main">
              <strong>{a.name}</strong>{' '}
              <span className="badge">{RUNTIME_LABELS[a.runtime] ?? a.runtime}</span>
              {a.description && <p className="project-desc">{a.description}</p>}
              {a.skills.length > 0 && (
                <p data-testid={`agent-skills-${a.id}`}>
                  技能: {a.skills.map((s) => (
                    <span key={s} className="badge">
                      {s}
                    </span>
                  ))}
                </p>
              )}
            </div>
            <div>
              <button data-testid={`edit-${a.id}`} onClick={() => startEdit(a)}>
                编辑
              </button>
              <button data-testid={`delete-${a.id}`} onClick={() => onDelete(a)}>
                删除
              </button>
            </div>
            {editingId === a.id && (
              <AgentEditor
                agent={a}
                name={editName}
                description={editDescription}
                skills={editSkills}
                onName={setEditName}
                onDescription={setEditDescription}
                onSkills={setEditSkills}
                onSave={onSaveEdit}
                onCancel={() => setEditingId('')}
              />
            )}
          </li>
        ))}
      </ul>
    </div>
    </div>
  )
}
