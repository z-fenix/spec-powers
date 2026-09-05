import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Modal } from '../components/Modal'
import { listAgents } from '../api/agents'
import { apiFetch } from '../api/client'
import {
  createAutopilot,
  deleteAutopilot,
  listAutopilots,
  triggerAutopilot,
  updateAutopilot,
  type Autopilot,
  type AutopilotInput,
} from '../api/autopilots'

interface ProjectOption {
  id: string
  name: string
}

const TRIGGER_LABELS: Record<Autopilot['trigger_type'], string> = {
  cron: '定时',
  webhook: 'Webhook',
  manual: '手动',
}

const ACTION_LABELS: Record<Autopilot['action_type'], string> = {
  create_issue: '创建 Issue',
  run_agent: '运行 Agent',
}

function emptyInput(): AutopilotInput {
  return {
    name: '',
    trigger_type: 'manual',
    cron_spec: '',
    webhook_id: '',
    action_type: 'create_issue',
    agent_id: '',
    project_id: '',
    issue_id: '',
    issue_title: '',
    issue_description: '',
    enabled: true,
  }
}

export function AutopilotsPage() {
  const [autopilots, setAutopilots] = useState<Autopilot[] | null>(null)
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [input, setInput] = useState<AutopilotInput>(emptyInput)
  const [webhookOptions, setWebhookOptions] = useState<{ id: string; name: string }[]>([])
  const [projectOptions, setProjectOptions] = useState<ProjectOption[]>([])
  const [agentOptions, setAgentOptions] = useState<{ id: string; name: string }[]>([])

  const reload = useCallback(() => {
    listAutopilots()
      .then(setAutopilots)
      .catch(() => {
        setError('加载失败')
        setAutopilots([])
      })
  }, [])

  useEffect(() => {
    reload()
    apiFetch<{ webhooks: { id: string; name: string }[] }>('/webhooks')
      .then((res) => setWebhookOptions(res.webhooks ?? []))
      .catch(() => setWebhookOptions([]))
    apiFetch<{ projects: ProjectOption[] }>('/projects')
      .then((res) => setProjectOptions(res.projects ?? []))
      .catch(() => setProjectOptions([]))
    listAgents()
      .then(setAgentOptions)
      .catch(() => setAgentOptions([]))
  }, [reload])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await createAutopilot(input)
      setShowCreate(false)
      setInput(emptyInput())
      reload()
    } catch {
      setError('创建失败')
    }
  }

  const onToggle = async (a: Autopilot) => {
    try {
      await updateAutopilot(a.id, {
        name: a.name,
        trigger_type: a.trigger_type,
        cron_spec: a.cron_spec,
        webhook_id: a.webhook_id,
        action_type: a.action_type,
        agent_id: a.agent_id,
        project_id: a.project_id,
        issue_id: a.issue_id,
        issue_title: a.issue_title,
        issue_description: a.issue_description,
        enabled: !a.enabled,
      })
      reload()
    } catch {
      setError('操作失败')
    }
  }

  const onTrigger = async (a: Autopilot) => {
    try {
      await triggerAutopilot(a.id)
      reload()
    } catch {
      setError('触发失败')
    }
  }

  const onDelete = async (a: Autopilot) => {
    try {
      await deleteAutopilot(a.id)
      reload()
    } catch {
      setError('删除失败')
    }
  }

  return (
    <div className="page">
      <div className="page-header">
        <h1 className="page-title">Autopilots</h1>
        <button type="button" className="btn btn-primary" data-testid="autopilot-create" onClick={() => setShowCreate(true)}>
          新建 Autopilot
        </button>
      </div>
      {error && <p role="alert">{error}</p>}
      {!error && autopilots === null && <p>加载中…</p>}
      {autopilots && autopilots.length === 0 && <p>暂无 Autopilot。</p>}
      <ul className="autopilot-list">
        {(autopilots ?? []).map((a) => (
          <li key={a.id} data-testid={`autopilot-row-${a.id}`}>
            <span className="autopilot-name">{a.name}</span>
            <span className="badge">
              {TRIGGER_LABELS[a.trigger_type]}
              {a.trigger_type === 'cron' && a.cron_spec ? ` · ${a.cron_spec}` : ''}
            </span>
            <span className="badge">{ACTION_LABELS[a.action_type]}</span>
            <span className="badge">{a.enabled ? '启用' : '停用'}</span>
            <button type="button" data-testid={`autopilot-trigger-${a.id}`} onClick={() => onTrigger(a)}>
              手动触发
            </button>
            <button type="button" data-testid={`autopilot-toggle-${a.id}`} onClick={() => onToggle(a)}>
              {a.enabled ? '停用' : '启用'}
            </button>
            <button type="button" data-testid={`autopilot-delete-${a.id}`} onClick={() => onDelete(a)}>
              删除
            </button>
          </li>
        ))}
      </ul>
      {showCreate && (
        <Modal title="新建 Autopilot" description="配置触发源与动作。" onClose={() => setShowCreate(false)}>
          <form onSubmit={submit}>
            <label>
              名称
              <input
                className="input"
                data-testid="autopilot-name"
                value={input.name}
                onChange={(e) => setInput({ ...input, name: e.target.value })}
                required
                autoFocus
              />
            </label>
            <label>
              触发源
              <select
                data-testid="autopilot-trigger-type"
                value={input.trigger_type}
                onChange={(e) => setInput({ ...input, trigger_type: e.target.value as AutopilotInput['trigger_type'] })}
              >
                <option value="manual">手动</option>
                <option value="cron">定时</option>
                <option value="webhook">Webhook</option>
              </select>
            </label>
            {input.trigger_type === 'cron' && (
              <label>
                Cron 表达式
                <input
                  className="input"
                  data-testid="autopilot-cron"
                  placeholder="0 9 * * 1-5"
                  value={input.cron_spec}
                  onChange={(e) => setInput({ ...input, cron_spec: e.target.value })}
                />
              </label>
            )}
            {input.trigger_type === 'webhook' && (
              <label>
                Webhook
                <select
                  data-testid="autopilot-webhook"
                  value={input.webhook_id}
                  onChange={(e) => setInput({ ...input, webhook_id: e.target.value })}
                >
                  <option value="">选择 webhook</option>
                  {webhookOptions.map((w) => (
                    <option key={w.id} value={w.id}>
                      {w.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              动作
              <select
                data-testid="autopilot-action-type"
                value={input.action_type}
                onChange={(e) => setInput({ ...input, action_type: e.target.value as AutopilotInput['action_type'] })}
              >
                <option value="create_issue">创建 Issue</option>
                <option value="run_agent">运行 Agent</option>
              </select>
            </label>
            {input.action_type === 'create_issue' ? (
              <>
                <label>
                  项目
                  <select
                    data-testid="autopilot-project"
                    value={input.project_id}
                    onChange={(e) => setInput({ ...input, project_id: e.target.value })}
                  >
                    <option value="">选择项目</option>
                    {projectOptions.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Issue 标题
                  <input
                    className="input"
                    data-testid="autopilot-issue-title"
                    value={input.issue_title}
                    onChange={(e) => setInput({ ...input, issue_title: e.target.value })}
                  />
                </label>
                <label>
                  Issue 描述
                  <textarea
                    data-testid="autopilot-issue-description"
                    value={input.issue_description}
                    onChange={(e) => setInput({ ...input, issue_description: e.target.value })}
                  />
                </label>
              </>
            ) : (
              <>
                <label>
                  Agent
                  <select
                    data-testid="autopilot-agent"
                    value={input.agent_id}
                    onChange={(e) => setInput({ ...input, agent_id: e.target.value })}
                  >
                    <option value="">选择 agent</option>
                    {agentOptions.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Issue ID
                  <input
                    className="input"
                    data-testid="autopilot-issue-id"
                    value={input.issue_id}
                    onChange={(e) => setInput({ ...input, issue_id: e.target.value })}
                  />
                </label>
              </>
            )}
            <button type="submit" className="btn btn-primary" data-testid="autopilot-submit">
              创建
            </button>
          </form>
        </Modal>
      )}
    </div>
  )
}
