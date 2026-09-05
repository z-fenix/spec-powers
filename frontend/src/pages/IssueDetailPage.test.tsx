import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { IssueDetailPage } from './IssueDetailPage'
import * as api from '../api/issues'
import * as workflowApi from '../api/workflow'
import * as runsApi from '../api/runs'
import type { Issue } from '../api/issues'
import { ApiError } from '../api/client'

vi.mock('../api/runs', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/runs')>()
  return {
    ...actual,
    getIssueUsage: vi.fn(),
  }
})

vi.mock('../api/workflow', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workflow')>()
  return {
    ...actual,
    getChangeByIssue: vi.fn(),
  }
})

vi.mock('../api/issues', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/issues')>()
  return {
    ...actual,
    getIssue: vi.fn(),
    updateIssue: vi.fn(),
    transitionIssue: vi.fn(),
    listChildren: vi.fn(),
    listComments: vi.fn(),
    listIssueEvents: vi.fn(),
    addComment: vi.fn(),
    listAttachments: vi.fn(),
    uploadAttachment: vi.fn(),
    downloadAttachment: vi.fn(),
    listMetadata: vi.fn(),
    setMetadata: vi.fn(),
    deleteMetadata: vi.fn(),
    listSubscribers: vi.fn(),
    addSubscriber: vi.fn(),
    removeSubscriber: vi.fn(),
  }
})

vi.mock('../api/properties', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/properties')>()
  return {
    ...actual,
    listPropertyDefinitions: vi.fn(),
    listIssueProperties: vi.fn(),
    setIssueProperty: vi.fn(),
  }
})

import * as propertiesApi from '../api/properties'

const mocked = vi.mocked(api)

const issue: Issue = {
  id: 'i1',
  project_id: 'p1',
  parent_id: '',
  title: 'parent issue',
  description: 'do the thing',
  status: 'todo',
  priority: 'none',
  assignee_id: 'u9',
  due_date: '2026-09-10',
  labels: [],
  stage: 2,
  position: 0,
  created_by: 'u1',
}

const child: Issue = {
  ...issue,
  id: 'c1',
  parent_id: 'i1',
  title: 'child task',
  stage: 1,
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/projects/p1/issues/i1']}>
      <Routes>
        <Route path="/projects/:id/issues/:issueId" element={<IssueDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(workflowApi.getChangeByIssue).mockRejectedValue(
    new ApiError(404, 'not_found', 'no change'),
  )
  vi.mocked(runsApi.getIssueUsage).mockResolvedValue({
    calls: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
  })
  mocked.getIssue.mockResolvedValue(issue)
  mocked.listChildren.mockResolvedValue([])
  mocked.listComments.mockResolvedValue([])
  mocked.listAttachments.mockResolvedValue([])
  mocked.listMetadata.mockResolvedValue([])
  mocked.listSubscribers.mockResolvedValue([])
  vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([])
  vi.mocked(propertiesApi.listIssueProperties).mockResolvedValue([])
  mocked.listIssueEvents.mockResolvedValue([])
})

describe('IssueDetailPage', () => {
  it('loads and renders issue fields', async () => {
    renderPage()

    expect(await screen.findByText('parent issue')).toBeInTheDocument()
    expect(screen.getByText('do the thing')).toBeInTheDocument()
    expect(screen.getByText('u9')).toBeInTheDocument()
    expect(screen.getByText('2026-09-10')).toBeInTheDocument()
  })

  it('shows the aggregated LLM usage', async () => {
    vi.mocked(runsApi.getIssueUsage).mockResolvedValue({
      calls: 2,
      prompt_tokens: 1234,
      completion_tokens: 567,
    })
    renderPage()

    const panel = await screen.findByTestId('issue-usage')
    expect(within(panel).getByTestId('usage-calls')).toHaveTextContent('2')
    expect(within(panel).getByTestId('usage-prompt')).toHaveTextContent('1,234')
    expect(within(panel).getByTestId('usage-completion')).toHaveTextContent('567')
    expect(runsApi.getIssueUsage).toHaveBeenCalledWith('i1')
  })

  it('renders zero usage without recorded runs', async () => {
    renderPage()

    const panel = await screen.findByTestId('issue-usage')
    expect(within(panel).getByTestId('usage-calls')).toHaveTextContent('0')
  it('renders the change timeline', async () => {
    mocked.listIssueEvents.mockResolvedValue([
      {
        id: 'e1',
        issue_id: 'i1',
        actor_id: 'u9',
        field: 'created',
        old_value: '',
        new_value: 'parent issue',
        created_at: '2026-09-05T00:00:00Z',
      },
      {
        id: 'e2',
        issue_id: 'i1',
        actor_id: 'u2',
        field: 'status',
        old_value: 'todo',
        new_value: 'in_progress',
        created_at: '2026-09-05T01:00:00Z',
      },
    ])
    renderPage()

    const timeline = await screen.findByTestId('timeline')
    expect(within(timeline).getByTestId('timeline-event-created')).toBeInTheDocument()
    expect(within(timeline).getByTestId('timeline-event-status')).toBeInTheDocument()
    expect(within(timeline).getByTestId('timeline-change-status')).toHaveTextContent(
      'todo → in_progress',
    )
    expect(within(timeline).getByTestId('timeline-actor-status')).toHaveTextContent('u2')
  })

  it('shows the empty timeline state', async () => {
    renderPage()

    expect(await screen.findByTestId('timeline-empty')).toBeInTheDocument()
  })

  it('renders the subtask tree', async () => {
    mocked.listChildren.mockResolvedValue([child])
    renderPage()

    const tree = await screen.findByTestId('subtask-tree')
    expect(within(tree).getByText('child task')).toBeInTheDocument()
    expect(within(tree).getByTestId('subtask-status-c1')).toHaveTextContent('待办')
  })

  it('transitions the issue status', async () => {
    mocked.transitionIssue.mockResolvedValue({ ...issue, status: 'in_progress' })
    renderPage()

    await userEvent.selectOptions(await screen.findByTestId('detail-status'), 'in_progress')

    expect(mocked.transitionIssue).toHaveBeenCalledWith('p1', 'i1', 'in_progress')
    expect(await screen.findByTestId('detail-status')).toHaveValue('in_progress')
  })

  it('edits the issue description', async () => {
    mocked.updateIssue.mockResolvedValue({ ...issue, description: 'do the thing appended' })
    renderPage()

    await userEvent.click(await screen.findByTestId('edit-issue'))
    await userEvent.type(screen.getByTestId('edit-description'), ' appended')
    await userEvent.click(screen.getByTestId('save-issue'))

    expect(mocked.updateIssue).toHaveBeenCalledWith('p1', 'i1', {
      title: 'parent issue',
      description: 'do the thing appended',
    })
    expect(await screen.findByText('do the thing appended')).toBeInTheDocument()
  })

  it('lists comments as threads and replies to one', async () => {
    const cm1 = { id: 'cm1', issue_id: 'i1', parent_id: '', author_id: 'u2', content: 'root comment', created_at: '' }
    const cm2 = { id: 'cm2', issue_id: 'i1', parent_id: 'cm1', author_id: 'u3', content: 'a reply', created_at: '' }
    mocked.listComments.mockResolvedValueOnce([cm1, cm2])
    mocked.addComment.mockResolvedValue({
      id: 'cm3',
      issue_id: 'i1',
      parent_id: 'cm1',
      author_id: 'me',
      content: 'my reply',
      created_at: '',
    })
    mocked.listComments.mockResolvedValueOnce([cm1, cm2, { ...cm2, id: 'cm3', author_id: 'me', content: 'my reply' }])
    renderPage()

    const thread = await screen.findByTestId('thread-cm1')
    expect(within(thread).getByText('root comment')).toBeInTheDocument()
    expect(within(thread).getByText('a reply')).toBeInTheDocument()

    await userEvent.type(within(thread).getByTestId('reply-input-cm1'), 'my reply')
    await userEvent.click(within(thread).getByTestId('reply-submit-cm1'))

    expect(mocked.addComment).toHaveBeenCalledWith('p1', 'i1', 'my reply', 'cm1')
    expect(await within(thread).findByText('my reply')).toBeInTheDocument()
  })

  it('renders comment content as markdown', async () => {
    const cm1 = {
      id: 'cm1',
      issue_id: 'i1',
      parent_id: '',
      author_id: 'u2',
      content: '# 标题\n\n**加粗** 内容',
      created_at: '',
    }
    mocked.listComments.mockResolvedValueOnce([cm1])
    renderPage()

    const thread = await screen.findByTestId('thread-cm1')
    expect(within(thread).getByRole('heading', { level: 1, name: '标题' })).toBeInTheDocument()
    expect(within(thread).getByText('加粗', { exact: false })).toBeInTheDocument()
  })

  it('sanitizes script tags in comment content', async () => {
    const cm1 = {
      id: 'cm1',
      issue_id: 'i1',
      parent_id: '',
      author_id: 'u2',
      content: '安全内容<script>alert(1)</script>',
      created_at: '',
    }
    mocked.listComments.mockResolvedValueOnce([cm1])
    const { container } = renderPage()

    const thread = await screen.findByTestId('thread-cm1')
    expect(within(thread).getByText('安全内容')).toBeInTheDocument()
    expect(container.querySelector('script')).toBeNull()
  })

  it('sanitizes script tags in reply content', async () => {
    const cm1 = { id: 'cm1', issue_id: 'i1', parent_id: '', author_id: 'u2', content: 'root', created_at: '' }
    const cm2 = {
      id: 'cm2',
      issue_id: 'i1',
      parent_id: 'cm1',
      author_id: 'u3',
      content: '回复<script>alert(1)</script>正文',
      created_at: '',
    }
    mocked.listComments.mockResolvedValueOnce([cm1, cm2])
    const { container } = renderPage()

    const reply = await screen.findByTestId('reply-cm2')
    expect(within(reply).getByText('回复正文')).toBeInTheDocument()
    expect(container.querySelector('script')).toBeNull()
  })

  it('adds a top-level comment', async () => {
    mocked.listComments.mockResolvedValueOnce([])
    mocked.addComment.mockResolvedValue({
      id: 'cm1',
      issue_id: 'i1',
      parent_id: '',
      author_id: 'me',
      content: 'new comment',
      created_at: '',
    })
    mocked.listComments.mockResolvedValueOnce([
      { id: 'cm1', issue_id: 'i1', parent_id: '', author_id: 'me', content: 'new comment', created_at: '' },
    ])
    renderPage()

    await userEvent.type(await screen.findByTestId('new-comment'), 'new comment')
    await userEvent.click(screen.getByTestId('submit-comment'))

    expect(mocked.addComment).toHaveBeenCalledWith('p1', 'i1', 'new comment', undefined)
    expect(await screen.findByTestId('thread-cm1')).toBeInTheDocument()
  })

  it('lists attachments and uploads one', async () => {
    const report = {
      id: 'a1',
      issue_id: 'i1',
      comment_id: '',
      file_name: 'report.txt',
      size_bytes: 5,
      content_type: 'text/plain',
      uploaded_by: 'u2',
      created_at: '',
    }
    mocked.listAttachments.mockResolvedValueOnce([report])
    mocked.uploadAttachment.mockResolvedValue({
      id: 'a2',
      issue_id: 'i1',
      comment_id: '',
      file_name: 'notes.txt',
      size_bytes: 3,
      content_type: 'text/plain',
      uploaded_by: 'me',
      created_at: '',
    })
    mocked.listAttachments.mockResolvedValueOnce([
      report,
      {
        id: 'a2',
        issue_id: 'i1',
        comment_id: '',
        file_name: 'notes.txt',
        size_bytes: 3,
        content_type: 'text/plain',
        uploaded_by: 'me',
        created_at: '',
      },
    ])
    renderPage()

    expect(await screen.findByText('report.txt')).toBeInTheDocument()

    const file = new File(['abc'], 'notes.txt', { type: 'text/plain' })
    const input = screen.getByTestId('attachment-input') as HTMLInputElement
    await userEvent.upload(input, file)

    expect(mocked.uploadAttachment).toHaveBeenCalledWith('p1', 'i1', file)
    expect(await screen.findByText('notes.txt')).toBeInTheDocument()
  })

  it('lists metadata and can set and delete an entry', async () => {
    const owner = { key: 'owner', value: 'alice', type: 'string', updated_at: '' }
    mocked.listMetadata.mockResolvedValueOnce([owner])
    mocked.setMetadata.mockResolvedValue({ key: 'k', value: 'v', type: 'string', updated_at: '' })
    mocked.listMetadata.mockResolvedValueOnce([owner, { key: 'k', value: 'v', type: 'string', updated_at: '' }])
    mocked.deleteMetadata.mockResolvedValue(undefined)
    renderPage()

    expect(await screen.findByText('alice')).toBeInTheDocument()

    await userEvent.type(screen.getByTestId('meta-key'), 'k')
    await userEvent.type(screen.getByTestId('meta-value'), 'v')
    await userEvent.click(screen.getByTestId('meta-set'))

    expect(mocked.setMetadata).toHaveBeenCalledWith('p1', 'i1', 'k', 'v', 'string')
    expect(await screen.findByTestId('meta-row-k')).toBeInTheDocument()

    await userEvent.click(screen.getByTestId('meta-delete-k'))
    expect(mocked.deleteMetadata).toHaveBeenCalledWith('p1', 'i1', 'k')
  })

  it('lists subscribers and can add and remove one', async () => {
    const bob = { user_id: 'u2', display_name: 'Bob', email: 'bob@example.com' }
    const carol = { user_id: 'u3', display_name: 'Carol', email: 'carol@example.com' }
    mocked.listSubscribers.mockResolvedValueOnce([bob])
    mocked.addSubscriber.mockResolvedValue([bob, carol])
    mocked.removeSubscriber.mockResolvedValue(undefined)
    mocked.listSubscribers.mockResolvedValueOnce([bob, carol])
    renderPage()

    const panel = await screen.findByTestId('subscribers')
    expect(within(panel).getByTestId('subscriber-u2')).toBeInTheDocument()

    await userEvent.type(within(panel).getByTestId('subscriber-email'), 'carol@example.com')
    await userEvent.click(within(panel).getByTestId('subscriber-add'))

    expect(mocked.addSubscriber).toHaveBeenCalledWith('p1', 'i1', 'carol@example.com')
    expect(await within(panel).findByTestId('subscriber-u3')).toBeInTheDocument()

    await userEvent.click(within(panel).getByTestId('subscriber-remove-u2'))
    expect(mocked.removeSubscriber).toHaveBeenCalledWith('p1', 'i1', 'u2')
  })

  it('shows an error when adding a subscriber fails', async () => {
    mocked.addSubscriber.mockRejectedValue(new Error('user not found'))
    renderPage()

    const panel = await screen.findByTestId('subscribers')
    await userEvent.type(within(panel).getByTestId('subscriber-email'), 'ghost@example.com')
    await userEvent.click(within(panel).getByTestId('subscriber-add'))

    expect(await within(panel).findByRole('alert')).toHaveTextContent('user not found')
  })

  it('shows the error message when loading fails', async () => {
    mocked.getIssue.mockRejectedValue(new Error('加载失败'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })

  it('hides the property panel when the project has no definitions', async () => {
    renderPage()

    await screen.findByTestId('issue-detail')
    expect(screen.queryByTestId('property-panel')).not.toBeInTheDocument()
  })

  it('shows and edits a select property', async () => {
    vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([
      {
        id: 'prop1',
        project_id: 'p1',
        name: '模块',
        type: 'select',
        options: ['前端', '后端'],
        position: 0,
      },
    ])
    vi.mocked(propertiesApi.listIssueProperties).mockResolvedValue([
      { issue_id: 'i1', property_id: 'prop1', value: '前端' },
    ])
    vi.mocked(propertiesApi.setIssueProperty).mockResolvedValue({
      issue_id: 'i1',
      property_id: 'prop1',
      value: '后端',
    })
    renderPage()

    const row = await screen.findByTestId('property-row-prop1')
    expect(within(row).getByText('模块')).toBeInTheDocument()
    expect(screen.getByTestId('property-input-prop1')).toHaveValue('前端')

    await userEvent.selectOptions(screen.getByTestId('property-input-prop1'), '后端')

    expect(propertiesApi.setIssueProperty).toHaveBeenCalledWith('p1', 'i1', 'prop1', '后端')
  })

  it('edits a text property and shows a validation error', async () => {
    vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([
      {
        id: 'prop2',
        project_id: 'p1',
        name: '备注',
        type: 'text',
        options: [],
        position: 0,
      },
    ])
    vi.mocked(propertiesApi.setIssueProperty).mockRejectedValue(
      new ApiError(400, 'invalid_request', 'bad value'),
    )
    renderPage()

    await screen.findByTestId('property-row-prop2')
    await userEvent.type(screen.getByTestId('property-input-prop2'), 'hello')
    fireEvent.blur(screen.getByTestId('property-input-prop2'))

    expect(propertiesApi.setIssueProperty).toHaveBeenCalledWith('p1', 'i1', 'prop2', 'hello')
    expect(await screen.findByRole('alert')).toBeInTheDocument()
  })

  it('toggles multi_select options as checkboxes', async () => {
    vi.mocked(propertiesApi.listPropertyDefinitions).mockResolvedValue([
      {
        id: 'prop3',
        project_id: 'p1',
        name: '标签',
        type: 'multi_select',
        options: ['x', 'y'],
        position: 0,
      },
    ])
    vi.mocked(propertiesApi.listIssueProperties).mockResolvedValue([
      { issue_id: 'i1', property_id: 'prop3', value: '["x"]' },
    ])
    vi.mocked(propertiesApi.setIssueProperty).mockResolvedValue({
      issue_id: 'i1',
      property_id: 'prop3',
      value: '["x","y"]',
    })
    renderPage()

    await screen.findByTestId('property-row-prop3')
    await userEvent.click(screen.getByLabelText('y'))

    expect(propertiesApi.setIssueProperty).toHaveBeenCalledWith('p1', 'i1', 'prop3', '["x","y"]')
  })
})
