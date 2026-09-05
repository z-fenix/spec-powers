import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addComment,
  deleteMetadata,
  downloadAttachment,
  getIssue,
  listAttachments,
  listChildren,
  listComments,
  listIssueEvents,
  listMetadata,
  setMetadata,
  transitionIssue,
  updateIssue,
  uploadAttachment,
  type Attachment,
  type Issue,
  type IssueComment,
  type IssueEvent,
  type MetadataEntry,
} from '../api/issues'
import { ApiError } from '../api/client'
import { STATUSES, STATUS_LABELS } from '../lib/status'
import { WorkflowProgress } from '../components/WorkflowProgress'
import { ArtifactViewer } from '../components/ArtifactViewer'
import { renderMarkdown } from '../lib/markdown'

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError || err instanceof Error ? err.message : fallback
}

function SubtaskTree({
  projectId,
  parentId,
  reloadKey,
}: {
  projectId: string
  parentId: string
  reloadKey: number
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
  if (children.length === 0) return null
  return (
    <ul className="subtask-list">
      {children.map((c) => (
        <li key={c.id} data-testid={`subtask-${c.id}`}>
          <Link to={`/projects/${projectId}/issues/${c.id}`}>{c.title}</Link>{' '}
          <span data-testid={`subtask-status-${c.id}`}>{STATUS_LABELS[c.status] ?? c.status}</span>
          <SubtaskTree projectId={projectId} parentId={c.id} reloadKey={reloadKey} />
        </li>
      ))}
    </ul>
  )
}

function CommentThread({
  comment,
  replies,
  onReply,
}: {
  comment: IssueComment
  replies: IssueComment[]
  onReply: (parentId: string, content: string) => Promise<void>
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
      <input
        data-testid={`reply-input-${comment.id}`}
        placeholder="回复…"
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      <button data-testid={`reply-submit-${comment.id}`} onClick={submit}>
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
            <button data-testid={`meta-delete-${m.key}`} onClick={() => onDelete(m.key)}>
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
      <button data-testid="meta-set" onClick={onSet}>
        设置
      </button>
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
            <button data-testid={`download-${a.id}`} onClick={() => onDownload(a)}>
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

export function IssueDetailPage() {
  const { id = '', issueId = '' } = useParams()
  const [issue, setIssue] = useState<Issue | null>(null)
  const [comments, setComments] = useState<IssueComment[] | null>(null)
  const [attachments, setAttachments] = useState<Attachment[] | null>(null)
  const [metadata, setMetadataList] = useState<MetadataEntry[] | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [newComment, setNewComment] = useState('')

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

  useEffect(() => {
    setIssue(null)
    setError('')
    loadIssue()
    loadComments()
    loadAttachments()
    loadMetadata()
  }, [loadIssue, loadComments, loadAttachments, loadMetadata])

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
  if (!issue || comments === null || attachments === null || metadata === null) {
    return <p>加载中…</p>
  }

  const roots = comments.filter((c) => !c.parent_id)
  const repliesOf = (parentId: string) => comments.filter((c) => c.parent_id === parentId)

  return (
    <section data-testid="issue-detail">
      <h2>{issue.title}</h2>
      {error && (
        <p role="alert" data-testid="detail-error">
          {error}
        </p>
      )}
      <div className="issue-meta">
        <span className="badge">S{issue.stage}</span>
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
        <span>
          负责人: <span data-testid="assignee">{issue.assignee_id || '未指派'}</span>
        </span>
        {issue.due_date && (
          <span>
            截止: <span data-testid="due-date">{issue.due_date}</span>
          </span>
        )}
        {!editing && (
          <button data-testid="edit-issue" onClick={startEdit}>
            编辑
          </button>
        )}
      </div>

      {editing ? (
        <div className="edit-form">
          <input
            data-testid="edit-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <textarea
            data-testid="edit-description"
            rows={6}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
          <button data-testid="save-issue" onClick={onSave}>
            保存
          </button>
        </div>
      ) : (
        <p className="issue-description">{issue.description}</p>
      )}

      <div data-testid="subtask-tree">
        <h3>子任务</h3>
        <SubtaskTree projectId={id} parentId={issueId} reloadKey={1} />
      </div>

      <div data-testid="timeline-section">
        <h3>时间线</h3>
        <TimelinePanel projectId={id} issueId={issueId} />
      </div>

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
  )
}
