import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { IssueDetailPage } from './IssueDetailPage'
import * as api from '../api/issues'
import type { Issue } from '../api/issues'

vi.mock('../api/issues', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/issues')>()
  return {
    ...actual,
    getIssue: vi.fn(),
    updateIssue: vi.fn(),
    transitionIssue: vi.fn(),
    listChildren: vi.fn(),
    listComments: vi.fn(),
    addComment: vi.fn(),
    listAttachments: vi.fn(),
    uploadAttachment: vi.fn(),
    downloadAttachment: vi.fn(),
    listMetadata: vi.fn(),
    setMetadata: vi.fn(),
    deleteMetadata: vi.fn(),
  }
})

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
  mocked.getIssue.mockResolvedValue(issue)
  mocked.listChildren.mockResolvedValue([])
  mocked.listComments.mockResolvedValue([])
  mocked.listAttachments.mockResolvedValue([])
  mocked.listMetadata.mockResolvedValue([])
})

describe('IssueDetailPage', () => {
  it('loads and renders issue fields', async () => {
    renderPage()

    expect(await screen.findByText('parent issue')).toBeInTheDocument()
    expect(screen.getByText('do the thing')).toBeInTheDocument()
    expect(screen.getByText('u9')).toBeInTheDocument()
    expect(screen.getByText('2026-09-10')).toBeInTheDocument()
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

  it('shows the error message when loading fails', async () => {
    mocked.getIssue.mockRejectedValue(new Error('加载失败'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('加载失败')
  })
})
