import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  listIssues,
  getIssue,
  createIssue,
  updateIssue,
  deleteIssue,
  transitionIssue,
  listChildren,
  listComments,
  addComment,
  listAttachments,
  uploadAttachment,
  downloadAttachment,
  listMetadata,
  setMetadata,
  deleteMetadata,
} from './issues'
import { apiFetch, setToken } from './client'

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

const issue = {
  id: 'i1',
  project_id: 'p1',
  parent_id: '',
  title: 't',
  description: '',
  status: 'todo',
  priority: 'none',
  assignee_id: '',
  due_date: '',
  labels: [],
  stage: 0,
  position: 0,
  created_by: 'u1',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('issues api', () => {
  it('lists issues with filters as query params', async () => {
    mockedFetch.mockResolvedValueOnce({ issues: [issue] })
    const res = await listIssues('p1', { status: 'todo', stage: 2, parent: 'root' })

    expect(apiFetch).toHaveBeenCalledWith(
      '/projects/p1/issues?status=todo&stage=2&parent=root',
    )
    expect(res).toEqual([issue])
  })

  it('lists issues without filters', async () => {
    mockedFetch.mockResolvedValueOnce({ issues: [] })
    await listIssues('p1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues')
  })

  it('gets one issue', async () => {
    mockedFetch.mockResolvedValueOnce({ issue })
    const res = await getIssue('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1')
    expect(res).toEqual(issue)
  })

  it('creates an issue with the given input', async () => {
    mockedFetch.mockResolvedValueOnce({ issue })
    await createIssue('p1', { title: 't', stage: 1 })

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues', {
      method: 'POST',
      body: { title: 't', stage: 1 },
    })
  })

  it('patches an issue with only provided fields', async () => {
    mockedFetch.mockResolvedValueOnce({ issue })
    await updateIssue('p1', 'i1', { title: 'n' })

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1', {
      method: 'PATCH',
      body: { title: 'n' },
    })
  })

  it('deletes an issue', async () => {
    mockedFetch.mockResolvedValueOnce(undefined)
    await deleteIssue('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1', { method: 'DELETE' })
  })

  it('transitions status', async () => {
    mockedFetch.mockResolvedValueOnce({ issue })
    await transitionIssue('p1', 'i1', 'in_progress')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/status', {
      method: 'POST',
      body: { status: 'in_progress' },
    })
  })

  it('lists children', async () => {
    mockedFetch.mockResolvedValueOnce({ issues: [issue] })
    const res = await listChildren('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/children')
    expect(res).toEqual([issue])
  })

  it('lists comments', async () => {
    const comment = { id: 'c1', issue_id: 'i1', parent_id: '', author_id: 'u1', content: 'hi', created_at: '' }
    mockedFetch.mockResolvedValueOnce({ comments: [comment] })
    const res = await listComments('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/comments')
    expect(res).toEqual([comment])
  })

  it('adds a comment, optionally as a reply', async () => {
    mockedFetch.mockResolvedValueOnce({ comment: { id: 'c1' } })
    await addComment('p1', 'i1', 'hi', 'c0')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/comments', {
      method: 'POST',
      body: { content: 'hi', parent_id: 'c0' },
    })
  })

  it('lists attachments', async () => {
    mockedFetch.mockResolvedValueOnce({ attachments: [] })
    const res = await listAttachments('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/attachments')
    expect(res).toEqual([])
  })

  it('uploads an attachment as multipart with the auth token', async () => {
    setToken('tok')
    const send = vi.fn(
      (_url: RequestInfo | URL, _init: RequestInit) =>
        Promise.resolve(new Response(JSON.stringify({ attachment: { id: 'a1' } }), { status: 201 })),
    )
    vi.stubGlobal('fetch', send)

    const file = new File(['data'], 'a.txt', { type: 'text/plain' })
    const res = await uploadAttachment('p1', 'i1', file)

    const [url, init] = send.mock.calls[0]
    expect(url).toBe('/api/v1/projects/p1/issues/i1/attachments')
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok')
    const form = init.body as FormData
    expect(form.get('file')).toBe(file)
    expect(res).toEqual({ id: 'a1' })
  })

  it('downloads an attachment as a blob with the auth token', async () => {
    setToken('tok')
    const send = vi.fn(
      (_url: RequestInfo | URL, _init: RequestInit) => Promise.resolve(new Response('bytes', { status: 200 })),
    )
    vi.stubGlobal('fetch', send)

    const blob = await downloadAttachment('p1', 'i1', 'a1')

    const [url, init] = send.mock.calls[0]
    expect(url).toBe('/api/v1/projects/p1/issues/i1/attachments/a1')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok')
    expect(blob).toBeInstanceOf(Blob)
  })

  it('lists metadata', async () => {
    mockedFetch.mockResolvedValueOnce({ metadata: [] })
    const res = await listMetadata('p1', 'i1')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/metadata')
    expect(res).toEqual([])
  })

  it('sets a metadata entry', async () => {
    mockedFetch.mockResolvedValueOnce({ entry: { key: 'k', value: 'v', type: 'string' } })
    await setMetadata('p1', 'i1', 'k', 'v', 'string')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/metadata/k', {
      method: 'PUT',
      body: { value: 'v', type: 'string' },
    })
  })

  it('deletes a metadata entry', async () => {
    mockedFetch.mockResolvedValueOnce(undefined)
    await deleteMetadata('p1', 'i1', 'k')

    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/metadata/k', { method: 'DELETE' })
  })
})
