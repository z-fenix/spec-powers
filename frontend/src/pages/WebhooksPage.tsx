import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Modal } from '../components/Modal'
import {
  createWebhook,
  deleteWebhook,
  listWebhooks,
  updateWebhook,
  type Webhook,
} from '../api/webhooks'

export function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[] | null>(null)
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')

  const reload = useCallback(() => {
    listWebhooks()
      .then(setWebhooks)
      .catch(() => {
        setError('加载失败')
        setWebhooks([])
      })
  }, [])

  useEffect(() => {
    let cancelled = false
    listWebhooks()
      .then((list) => {
        if (!cancelled) setWebhooks(list)
      })
      .catch(() => {
        if (!cancelled) {
          setError('加载失败')
          setWebhooks([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await createWebhook(name)
      setShowCreate(false)
      setName('')
      reload()
    } catch {
      setError('创建失败')
    }
  }

  const onToggle = async (w: Webhook) => {
    try {
      await updateWebhook(w.id, { enabled: !w.enabled })
      reload()
    } catch {
      setError('操作失败')
    }
  }

  const onDelete = async (w: Webhook) => {
    try {
      await deleteWebhook(w.id)
      reload()
    } catch {
      setError('删除失败')
    }
  }

  return (
    <div className="page">
      <div className="page-header">
        <h1 className="page-title">Webhooks</h1>
        <button type="button" className="btn btn-primary" data-testid="webhook-create" onClick={() => setShowCreate(true)}>
          新建 Webhook
        </button>
      </div>
      {error && <p role="alert">{error}</p>}
      {!error && webhooks === null && <p>加载中…</p>}
      {webhooks && webhooks.length === 0 && <p>暂无 Webhook。</p>}
      <ul className="webhook-list">
        {(webhooks ?? []).map((w) => (
          <li key={w.id} data-testid={`webhook-row-${w.id}`}>
            <span className="webhook-name">{w.name}</span>
            <span className="badge">{w.enabled ? '启用' : '停用'}</span>
            <code className="webhook-secret">{w.secret}</code>
            <code className="webhook-url">POST /api/v1/hooks/{w.id}</code>
            <button type="button" data-testid={`webhook-toggle-${w.id}`} onClick={() => onToggle(w)}>
              {w.enabled ? '停用' : '启用'}
            </button>
            <button type="button" data-testid={`webhook-delete-${w.id}`} onClick={() => onDelete(w)}>
              删除
            </button>
          </li>
        ))}
      </ul>
      {showCreate && (
        <Modal title="新建 Webhook" description="创建入站 webhook 端点，系统将生成签名密钥。" onClose={() => setShowCreate(false)}>
          <form onSubmit={onCreate}>
            <label>
              名称
              <input
                className="input"
                data-testid="webhook-name"
                placeholder="webhook 名称"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                autoFocus
              />
            </label>
            <button type="submit" className="btn btn-primary" data-testid="webhook-submit">
              创建
            </button>
          </form>
        </Modal>
      )}
    </div>
  )
}
