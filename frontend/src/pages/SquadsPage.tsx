import { useCallback, useEffect, useState } from 'react'
import { listAgents, type Agent } from '../api/agents'
import {
  addSquadMember,
  createSquad,
  deleteSquad,
  getSquad,
  listSquads,
  removeSquadMember,
  setSquadLeader,
  type Squad,
} from '../api/squads'

export function SquadsPage() {
  const [squads, setSquads] = useState<Squad[] | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [leaderId, setLeaderId] = useState('')
  const [creating, setCreating] = useState(false)

  const refresh = useCallback(async () => {
    const list = await listSquads()
    const detailed = await Promise.all(
      list.map(async (s) => {
        try {
          return await getSquad(s.id)
        } catch {
          return s
        }
      }),
    )
    setSquads(detailed)
  }, [])

  useEffect(() => {
    let cancelled = false
    Promise.all([refresh(), listAgents()])
      .then(([, agentList]) => {
        if (!cancelled) setAgents(agentList)
      })
      .catch(() => {
        if (!cancelled) {
          setError('加载失败')
          setSquads([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [refresh])

  const handleCreate = async () => {
    if (!name.trim() || !leaderId) return
    setCreating(true)
    try {
      await createSquad({ name: name.trim(), description: description.trim(), leader_id: leaderId })
      setName('')
      setDescription('')
      setLeaderId('')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleRemoveMember = async (squad: Squad, userId: string) => {
    try {
      await removeSquadMember(squad.id, userId)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : '移除失败')
    }
  }

  const handleAddMember = async (squad: Squad, userId: string) => {
    if (!userId) return
    try {
      await addSquadMember(squad.id, userId)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : '添加失败')
    }
  }

  const handleSetLeader = async (squad: Squad, userId: string) => {
    try {
      await setSquadLeader(squad.id, userId)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : '设置失败')
    }
  }

  const handleDelete = async (squad: Squad) => {
    try {
      await deleteSquad(squad.id)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败')
    }
  }

  if (error && squads === null) return <p role="alert">{error}</p>
  if (squads === null) return <p>加载中…</p>

  return (
    <section data-testid="squads-page">
      <h2>小组</h2>
      {error && (
        <p role="alert" data-testid="squads-error">
          {error}
        </p>
      )}

      <div className="squad-create" data-testid="squad-create">
        <h3>创建小组</h3>
        <input
          data-testid="squad-name-input"
          placeholder="小组名称"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          data-testid="squad-description-input"
          placeholder="描述（可选）"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <select data-testid="squad-leader-select" value={leaderId} onChange={(e) => setLeaderId(e.target.value)}>
          <option value="">选择 leader</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
        <button type="button" data-testid="squad-create-btn" disabled={creating} onClick={handleCreate}>
          创建
        </button>
      </div>

      {squads.length === 0 && <p>暂无小组。</p>}
      <ul className="squad-list">
        {squads.map((s) => {
          const leader = s.members?.find((m) => m.user_id === s.leader_id)
          const candidates = agents.filter((a) => !(s.members ?? []).some((m) => m.user_id === a.id))
          return (
            <li key={s.id} data-testid={`squad-row-${s.id}`}>
              <div className="squad-head">
                <span className="squad-name">{s.name}</span>
                {s.description && <span className="squad-description">{s.description}</span>}
                <span className="badge" data-testid={`squad-leader-${s.id}`}>
                  Leader: {leader?.display_name ?? s.leader_id}
                </span>
                <button
                  type="button"
                  className="btn btn-ghost"
                  data-testid={`squad-delete-${s.id}`}
                  onClick={() => handleDelete(s)}
                >
                  删除
                </button>
              </div>
              <ul className="squad-members" data-testid={`squad-members-${s.id}`}>
                {(s.members ?? []).map((m) => (
                  <li key={m.user_id} data-testid={`squad-member-${m.user_id}`}>
                    <span className="squad-member-name">{m.display_name}</span>
                    {m.is_agent && <span className="badge">agent</span>}
                    {m.user_id === s.leader_id && <span className="badge">leader</span>}
                    <button
                      type="button"
                      className="btn btn-ghost"
                      data-testid={`squad-remove-${m.user_id}`}
                      onClick={() => handleRemoveMember(s, m.user_id)}
                    >
                      移除
                    </button>
                  </li>
                ))}
              </ul>
              <div className="squad-actions">
                <select
                  data-testid={`squad-add-member-select-${s.id}`}
                  defaultValue=""
                  onChange={(e) => {
                    void handleAddMember(s, e.target.value)
                    e.target.value = ''
                  }}
                >
                  <option value="">添加成员…</option>
                  {candidates.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
                </select>
                <select
                  data-testid={`squad-set-leader-select-${s.id}`}
                  defaultValue=""
                  onChange={(e) => {
                    void handleSetLeader(s, e.target.value)
                    e.target.value = ''
                  }}
                >
                  <option value="">设为 leader…</option>
                  {(s.members ?? [])
                    .filter((m) => m.user_id !== s.leader_id)
                    .map((m) => (
                      <option key={m.user_id} value={m.user_id}>
                        {m.display_name}
                      </option>
                    ))}
                </select>
              </div>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
