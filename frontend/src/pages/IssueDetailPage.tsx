import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addComment,
  addSubscriber,
  deleteMetadata,
  downloadAttachment,
  getIssue,
  listAttachments,
  listChildren,
  listComments,
  listIssueEvents,
  listMetadata,
  listSubscribers,
  removeSubscriber,
  setMetadata,
  transitionIssue,
  updateIssue,
  uploadAttachment,
  type Attachment,
  type Issue,
  type IssueComment,
  type IssueEvent,
  type MetadataEntry,
  type Subscriber,
} from '../api/issues'
import { listAgents } from '../api/agents'
import { ApiError } from '../api/client'
import { STATUSES, STATUS_LABELS } from '../lib/status'
import {
  decodeMultiSelect,
  encodeMultiSelect,
  listIssueProperties,
  listPropertyDefinitions,
  setIssueProperty,
  type IssuePropertyValue,
  type PropertyDefinition,
} from '../api/properties'
import { StatusIcon } from '../components/StatusIcon'
import { WorkflowProgress } from '../components/WorkflowProgress'
import { ArtifactViewer } from '../components/ArtifactViewer'
import { IssueUsagePanel } from '../components/Usage'
import { MentionText } from '../components/MentionText'
import { MentionInput, type MentionCandidate } from '../components/MentionInput'
import { renderMarkdown } from '../lib/markdown'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

function SubtaskTree({
  projectId,
  parentId,
  reloadKey,
  path = [],
}: {
  projectId: string
  parentId: string
  reloadKey: number
  path?: string[]
}) {
  const [children, setChildren] = useState<Issue[] | null>(null)

  useEffect(() => {
    let cancelled = false
    listChildren(projectId, parentId)
      .then((list) => {
        if (!cancelled) setChildren(list)
      })
      .catch(() => {
        if (!cancelled) setChildren([])
      })
    return () => {
      cancelled = true
    }
  }, [projectId, parentId, reloadKey])

  if (children === null) return null
  const visibleChildren = children.filter((c) => !path.includes(c.id))
  if (visibleChildren.length === 0) return null
  return (
    <div className="detail-section" data-testid="subtask-tree">
      <h3>子任务</h3>
      <ul className="subtask-list">
      {visibleChildren.map((c) => (
        <li key={c.id} data-testid={`subtask-${c.id}`}>
          <Link to={`/projects/${projectId}/issues/${c.id}`}>{c.title}</Link>{' '}
          <span data-testid={`subtask-status-${c.id}`}>{STATUS_LABELS[c.status] ?? c.status}</span>
          <SubtaskTree
            projectId={projectId}
            parentId={c.id}
            reloadKey={reloadKey}
            path={[...path, c.id]}
          />
        </li>
      ))}
      </ul>
    </div>
  )
}

function CommentThread({
  comment,
  replies,
  onReply,
  candidates,
}: {
  comment: IssueComment
  replies: IssueComment[]
  onReply: (parentId: string, content: string) => Promise<void>
  candidates: MentionCandidate[]
}) {
  const [text, setText] = useState('')

  const submit = () => {
    if (!text.trim()) return
    onReply(comment.id, text).then(() => setText(''))
  }

  return (
    <div className="comment-thread" data-testid={`thread-${comment.id}`}>
      <div className="comment">
        <span className="comment-author">{comment.author_id}</span>
        <div
          className="comment-content markdown-body"
          dangerouslySetInnerHTML={{ __html: renderMarkdown(comment.content) }}
        />
      </div>
      <div className="comment-replies">
        {replies.map((r) => (
          <div key={r.id} className="comment reply" data-testid={`reply-${r.id}`}>
            <span className="comment-author">{r.author_id}</span>
            <div
              className="comment-content markdown-body"
              dangerouslySetInnerHTML={{ __html: renderMarkdown(r.content) }}
            />
          </div>
        ))}
      </div>
      <MentionInput
        testId={`reply-input-${comment.id}`}
        placeholder="回复…（@ 可提及成员或 Agent）"
        value={text}
        onChange={setText}
        candidates={candidates}
      />
      <button className="btn btn-primary btn-sm" data-testid={`reply-submit-${comment.id}`} onClick={submit}>
        回复
      </button>
    </div>
  )
}

const EVENT_FIELD_LABELS: Record<string, string> = {
  created: '创建',
  status: '状态',
  assignee: '负责人',
  title: '标题',
  description: '描述',
  priority: '优先级',
  due_date: '截止日',
  labels: '标签',
  parent: '父任务',
  stage: 'Stage',
  position: '位置',
}

function TimelinePanel({ projectId, issueId }: { projectId: string; issueId: string }) {
  const [events, setEvents] = useState<IssueEvent[] | null>(null)

  useEffect(() => {
    let cancelled = false
    listIssueEvents(projectId, issueId)
      .then((list) => {
        if (!cancelled) setEvents(list)
      })
      .catch(() => {
        if (!cancelled) setEvents([])
      })
    return () => {
      cancelled = true
    }
  }, [projectId, issueId])

  if (events === null) return <p data-testid="timeline-loading">加载中…</p>
  if (events.length === 0) return <p data-testid="timeline-empty">暂无变更记录。</p>
  return (
    <ul className="issue-timeline" data-testid="timeline">
      {events.map((e) => (
        <li key={e.id} data-testid={`timeline-event-${e.field}`}>
          <span className="badge">{EVENT_FIELD_LABELS[e.field] ?? e.field}</span>{' '}
          <span data-testid={`timeline-actor-${e.field}`}>{e.actor_id || '系统'}</span>
          {e.field !== 'created' && (
            <span data-testid={`timeline-change-${e.field}`}>
              {' '}
              {e.old_value || '(空)'} → {e.new_value || '(空)'}
            </span>
          )}
        </li>
      ))}
    </ul>
  )
}

function MetadataPanel({
  projectId,
  issueId,
  entries,
  onChanged,
}: {
  projectId: string
  issueId: string
  entries: MetadataEntry[]
  onChanged: () => void
}) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [error, setError] = useState('')

  const onSet = () => {
    setError('')
    setMetadata(projectId, issueId, key, value, 'string')
      .then(() => {
        setKey('')
        setValue('')
        onChanged()
      })
      .catch((err) => setError(errorMessage(err, '设置元数据失败')))
  }

  const onDelete = (k: string) => {
    setError('')
    deleteMetadata(projectId, issueId, k)
      .then(onChanged)
      .catch((err) => setError(errorMessage(err, '删除元数据失败')))
  }

  return (
    <div data-testid="metadata-panel">
      <h3>元数据</h3>
      {error && <p role="alert">{error}</p>}
      {entries.length === 0 && <p>暂无元数据。</p>}
      <ul>
        {entries.map((m) => (
          <li key={m.key} data-testid={`meta-row-${m.key}`}>
            <code>{m.key}</code> = <span data-testid={`meta-value-${m.key}`}>{m.value}</span>{' '}
            <button className="btn btn-ghost btn-sm" data-testid={`meta-delete-${m.key}`} onClick={() => onDelete(m.key)}>
              删除
            </button>
          </li>
        ))}
      </ul>
      <input
        data-testid="meta-key"
        placeholder="键"
        value={key}
        onChange={(e) => setKey(e.target.value)}
      />
      <input
        data-testid="meta-value"
        placeholder="值"
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      <button className="btn btn-outline btn-sm" data-testid="meta-set" onClick={onSet}>
        设置
      </button>
    </div>
  )
}

function PropertyValueEditor({
  projectId,
  issueId,
  def,
  value,
  onChanged,
}: {
  projectId: string
  issueId: string
  def: PropertyDefinition
  value: string
  onChanged: () => void
}) {
  const [error, setError] = useState('')
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])

  const save = (v: string) => {
    setError('')
    setIssueProperty(projectId, issueId, def.id, v)
      .then(onChanged)
      .catch((err) => setError(errorMessage(err, '保存属性失败')))
  }

  const testId = `property-input-${def.id}`
  let editor: ReactNode
  switch (def.type) {
    case 'select':
      editor = (
        <select
          data-testid={testId}
          value={value}
          onChange={(e) => save(e.target.value)}
        >
          <option value="">未设置</option>
          {def.options.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      )
      break
    case 'multi_select': {
      const picked = decodeMultiSelect(value)
      const toggle = (o: string, on: boolean) => {
        const next = on ? [...picked, o] : picked.filter((v) => v !== o)
        save(encodeMultiSelect(next))
      }
      editor = (
        <span data-testid={testId}>
          {def.options.map((o) => (
            <label key={o}>
              <input
                type="checkbox"
                checked={picked.includes(o)}
                onChange={(e) => toggle(o, e.target.checked)}
              />{' '}
              {o}
            </label>
          ))}
        </span>
      )
      break
    }
    case 'checkbox':
      editor = (
        <input
          data-testid={testId}
          type="checkbox"
          checked={value === 'true'}
          onChange={(e) => save(e.target.checked ? 'true' : 'false')}
        />
      )
      break
    case 'number':
      editor = (
        <input
          data-testid={testId}
          type="number"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => draft !== value && save(draft)}
        />
      )
      break
    case 'date':
      editor = (
        <input
          data-testid={testId}
          type="date"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => draft !== value && save(draft)}
        />
      )
      break
    default:
      editor = (
        <input
          data-testid={testId}
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => draft !== value && save(draft)}
        />
      )
  }

  return (
    <li data-testid={`property-row-${def.id}`}>
      <strong>{def.name}</strong>{' '}
      <span className="badge">{def.type}</span> {editor}
      {error && <span role="alert"> {error}</span>}
    </li>
  )
}

function PropertyValuesPanel({
  projectId,
  issueId,
}: {
  projectId: string
  issueId: string
}) {
  const [defs, setDefs] = useState<PropertyDefinition[] | null>(null)
  const [values, setValues] = useState<IssuePropertyValue[] | null>(null)

  const loadValues = useCallback(
    () =>
      listIssueProperties(projectId, issueId)
        .then(setValues)
        .catch(() => setValues([])),
    [projectId, issueId],
  )

  useEffect(() => {
    let cancelled = false
    listPropertyDefinitions(projectId)
      .then((list) => {
        if (!cancelled) setDefs(list ?? [])
      })
      .catch(() => {
        if (!cancelled) setDefs([])
      })
    loadValues()
    return () => {
      cancelled = true
    }
  }, [projectId, issueId, loadValues])

  if (defs !== null && defs.length === 0) return null
  if (defs === null || values === null) return null

  const valueOf = (propertyId: string) =>
    values.find((v) => v.property_id === propertyId)?.value ?? ''

  return (
    <div data-testid="property-panel">
      <h3>属性</h3>
      <ul>
        {defs.map((d) => (
          <PropertyValueEditor
            key={d.id}
            projectId={projectId}
            issueId={issueId}
            def={d}
            value={valueOf(d.id)}
            onChanged={loadValues}
          />
        ))}
      </ul>
    </div>
  )
}

function AttachmentPanel({
  projectId,
  issueId,
  attachments,
  onChanged,
}: {
  projectId: string
  issueId: string
  attachments: Attachment[]
  onChanged: () => void
}) {
  const [error, setError] = useState('')

  const onUpload = async (files: FileList | null) => {
    if (!files || files.length === 0) return
    setError('')
    try {
      await uploadAttachment(projectId, issueId, files[0])
      onChanged()
    } catch (err) {
      setError(errorMessage(err, '上传失败'))
    }
  }

  const onDownload = async (a: Attachment) => {
    setError('')
    try {
      const blob = await downloadAttachment(projectId, issueId, a.id)
      const url = URL.createObjectURL(blob)
      const el = document.createElement('a')
      el.href = url
      el.download = a.file_name
      el.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(errorMessage(err, '下载失败'))
    }
  }

  return (
    <div data-testid="attachments">
      <h3>附件</h3>
      {error && <p role="alert">{error}</p>}
      {attachments.length === 0 && <p>暂无附件。</p>}
      <ul>
        {attachments.map((a) => (
          <li key={a.id}>
            {a.file_name} <span className="badge">{a.content_type}</span>{' '}
            <button className="btn btn-outline btn-sm" data-testid={`download-${a.id}`} onClick={() => onDownload(a)}>
              下载
            </button>
          </li>
        ))}
      </ul>
      <input
        data-testid="attachment-input"
        type="file"
        onChange={(e) => onUpload(e.target.files)}
      />
    </div>
  )
}

function SubscribersPanel({
  projectId,
  issueId,
  subscribers,
  onChanged,
}: {
  projectId: string
  issueId: string
  subscribers: Subscriber[]
  onChanged: () => void
}) {
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')

  const onAdd = () => {
    if (!email.trim()) return
    setError('')
    addSubscriber(projectId, issueId, email)
      .then(() => {
        setEmail('')
        onChanged()
      })
      .catch((err) => setError(errorMessage(err, '添加订阅者失败')))
  }

  const onRemove = (userId: string) => {
    setError('')
    removeSubscriber(projectId, issueId, userId)
      .then(onChanged)
      .catch((err) => setError(errorMessage(err, '移除订阅者失败')))
  }

  return (
    <div data-testid="subscribers">
      <h3>订阅者</h3>
      {error && <p role="alert">{error}</p>}
      {subscribers.length === 0 && <p>暂无订阅者。</p>}
      <ul>
        {subscribers.map((s) => (
          <li key={s.user_id} data-testid={`subscriber-${s.user_id}`}>
            {s.display_name || s.email}{' '}
            <button
              className="btn btn-ghost btn-sm"
              data-testid={`subscriber-remove-${s.user_id}`}
              onClick={() => onRemove(s.user_id)}
            >
              移除
            </button>
          </li>
        ))}
      </ul>
      <input
        className="input"
        data-testid="subscriber-email"
        placeholder="邮箱"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <button className="btn btn-outline btn-sm" data-testid="subscriber-add" onClick={onAdd}>
        添加
      </button>
    </div>
  )
}

export function IssueDetailPage() {
  const { id = '', issueId = '' } = useParams()
  const [issue, setIssue] = useState<Issue | null>(null)
  const [comments, setComments] = useState<IssueComment[] | null>(null)
  const [attachments, setAttachments] = useState<Attachment[] | null>(null)
  const [metadata, setMetadataList] = useState<MetadataEntry[] | null>(null)
  const [subscribers, setSubscribers] = useState<Subscriber[] | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [newComment, setNewComment] = useState('')
  const [agents, setAgents] = useState<MentionCandidate[]>([])

  const loadIssue = useCallback(
    () =>
      getIssue(id, issueId)
        .then(setIssue)
        .catch((err) => setError(errorMessage(err, '加载失败'))),
    [id, issueId],
  )

  const loadComments = useCallback(
    () =>
      listComments(id, issueId)
        .then(setComments)
        .catch(() => setComments([])),
    [id, issueId],
  )

  const loadAttachments = useCallback(
    () =>
      listAttachments(id, issueId)
        .then(setAttachments)
        .catch(() => setAttachments([])),
    [id, issueId],
  )

  const loadMetadata = useCallback(
    () =>
      listMetadata(id, issueId)
        .then(setMetadataList)
        .catch(() => setMetadataList([])),
    [id, issueId],
  )

  const loadSubscribers = useCallback(
    () =>
      listSubscribers(id, issueId)
        .then(setSubscribers)
        .catch(() => setSubscribers([])),
    [id, issueId],
  )

  useEffect(() => {
    setIssue(null)
    setError('')
    loadIssue()
    loadComments()
    loadAttachments()
    loadMetadata()
    loadSubscribers()
  }, [loadIssue, loadComments, loadAttachments, loadMetadata, loadSubscribers])

  useEffect(() => {
    let cancelled = false
    listAgents()
      .then((list) => {
        if (!cancelled) {
          setAgents(
            list
              .filter((a) => a.name)
              .map((a) => ({ kind: 'agent' as const, id: a.id, name: a.name })),
          )
        }
      })
      .catch(() => {
        // mention completion is best-effort; agents stay empty on failure
      })
    return () => {
      cancelled = true
    }
  }, [])

  const onTransition = (status: string) => {
    setError('')
    transitionIssue(id, issueId, status)
      .then(setIssue)
      .catch((err) => setError(errorMessage(err, '状态流转失败')))
  }

  const startEdit = () => {
    if (!issue) return
    setTitle(issue.title)
    setDescription(issue.description)
    setEditing(true)
  }

  const onSave = () => {
    setError('')
    updateIssue(id, issueId, { title, description })
      .then((updated) => {
        setIssue(updated)
        setEditing(false)
      })
      .catch((err) => setError(errorMessage(err, '保存失败')))
  }

  const onSubmitComment = (e: FormEvent) => {
    e.preventDefault()
    if (!newComment.trim()) return
    setError('')
    addComment(id, issueId, newComment, undefined)
      .then(() => {
        setNewComment('')
        return loadComments()
      })
      .catch((err) => setError(errorMessage(err, '评论失败')))
  }

  const onReply = (parentId: string, content: string) =>
    addComment(id, issueId, content, parentId).then(() => loadComments())

  if (error && !issue) return <p role="alert">{error}</p>
  if (!issue || comments === null || attachments === null || metadata === null || subscribers === null) {
    return <p>加载中…</p>
  }

  const roots = comments.filter((c) => !c.parent_id)
  const repliesOf = (parentId: string) => comments.filter((c) => c.parent_id === parentId)

  // Member candidates come from comment authors; the backend exposes no
  // member-name API, so the author id doubles as the display name.
  const memberCandidates: MentionCandidate[] = []
  const seenAuthors = new Set<string>()
  for (const c of comments) {
    if (c.author_id && !seenAuthors.has(c.author_id)) {
      seenAuthors.add(c.author_id)
      memberCandidates.push({ kind: 'member', id: c.author_id, name: c.author_id })
    }
  }
  const candidates = [...agents, ...memberCandidates]

  return (
    <div className="page" data-testid="issue-detail">
      <div className="page-header">
        <nav className="breadcrumb">
          <Link to={`/projects/${id}/board`}>看板</Link>
          <span className="crumb-sep">›</span>
          <span className="crumb-leaf">#{issueId.slice(0, 8)}</span>
        </nav>
        <div className="page-header-actions">
          {!editing && (
            <button className="btn btn-ghost btn-sm" data-testid="edit-issue" onClick={startEdit}>
              编辑
            </button>
          )}
        </div>
      </div>
      <div className="detail-layout">
        <div className="detail-main">
          <div className="detail-main-inner">
            {error && (
              <p role="alert" data-testid="detail-error" style={{ color: 'var(--destructive)', marginTop: 0 }}>
                {error}
              </p>
            )}
            <h1 className="detail-title">{issue.title}</h1>
            {editing ? (
              <div className="edit-form" style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 8 }}>
                <input
                  className="input"
                  data-testid="edit-title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
                <textarea
                  className="input"
                  data-testid="edit-description"
                  rows={6}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
                <div style={{ display: 'flex', gap: 8 }}>
                  <button className="btn btn-primary btn-sm" data-testid="save-issue" onClick={onSave}>
                    保存
                  </button>
                </div>
              </div>
            ) : (
              <p className="detail-desc">{issue.description}</p>
            )}

            <SubtaskTree projectId={id} parentId={issueId} reloadKey={1} />

            <div className="detail-section">
              <WorkflowProgress issueId={issueId} />
              <ArtifactViewer issueId={issueId} />
            </div>

            <div className="detail-section">
              <h3>时间线</h3>
              <TimelinePanel projectId={id} issueId={issueId} />
              <IssueUsagePanel issueId={issueId} />
            </div>

            <div className="detail-section">
              <h3>评论</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {roots.map((c) => (
                  <CommentThread
                    key={c.id}
                    comment={c}
                    replies={repliesOf(c.id)}
                    onReply={onReply}
                    candidates={candidates}
                  />
                ))}
              </div>
              <form onSubmit={onSubmitComment} className="inline-form">
                <input
                  className="input"
                  style={{ flex: 1, minWidth: 200 }}
                  data-testid="new-comment"
                  placeholder="写评论…"
                  value={newComment}
                  onChange={(e) => setNewComment(e.target.value)}
                />
                <button type="submit" className="btn btn-primary btn-sm" data-testid="submit-comment">
                  评论
                </button>
              </form>
            </div>
          </div>
        </div>

        <aside className="detail-aside">
          <div>
            <p className="section-title">属性</p>
            <div className="prop-row">
              <span className="prop-label">状态</span>
              <span className="prop-value">
                <StatusIcon status={issue.status} />
                <select
                  aria-label="状态"
                  data-testid="detail-status"
                  value={issue.status}
                  onChange={(e) => onTransition(e.target.value)}
                >
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {STATUS_LABELS[s]}
                    </option>
                  ))}
                </select>
              </span>
            </div>
            <div className="prop-row">
              <span className="prop-label">Stage</span>
              <span className="prop-value">
                <span className="chip">S{issue.stage}</span>
              </span>
            </div>
            <div className="prop-row">
              <span className="prop-label">负责人</span>
              <span className="prop-value">
                <span data-testid="assignee">{issue.assignee_id || '未指派'}</span>
              </span>
            </div>
            {issue.due_date && (
              <div className="prop-row">
                <span className="prop-label">截止</span>
                <span className="prop-value">
                  <span data-testid="due-date">{issue.due_date}</span>
                </span>
              </div>
            )}
          </div>

          <AttachmentPanel
            projectId={id}
            issueId={issueId}
            attachments={attachments}
            onChanged={loadAttachments}
          />
          <SubscribersPanel
            projectId={id}
            issueId={issueId}
            subscribers={subscribers}
            onChanged={loadSubscribers}
          />
          <MetadataPanel
            projectId={id}
            issueId={issueId}
            entries={metadata}
            onChanged={loadMetadata}
          />
        </aside>
      </div>

      <PropertyValuesPanel projectId={id} issueId={issueId} />

      <WorkflowProgress issueId={issueId} />
      <ArtifactViewer issueId={issueId} />

      <h3>评论</h3>
      {roots.map((c) => (
        <CommentThread
          key={c.id}
          comment={c}
          replies={repliesOf(c.id)}
          onReply={onReply}
        />
      ))}
      <form onSubmit={onSubmitComment} className="inline-form">
        <input
          data-testid="new-comment"
          placeholder="写评论…"
          value={newComment}
          onChange={(e) => setNewComment(e.target.value)}
        />
        <button type="submit" data-testid="submit-comment">
          评论
        </button>
      </form>

      <AttachmentPanel
        projectId={id}
        issueId={issueId}
        attachments={attachments}
        onChanged={loadAttachments}
      />
      <MetadataPanel
        projectId={id}
        issueId={issueId}
        entries={metadata}
        onChanged={loadMetadata}
      />
    </section>
    </div>
  )
}
