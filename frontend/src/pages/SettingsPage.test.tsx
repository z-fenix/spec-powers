import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'
import * as api from '../api/workspace'
import type { Member, Invite, APIToken } from '../api/workspace'

vi.mock('../api/workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workspace')>()
  return {
    ...actual,
    listMembers: vi.fn(),
    inviteMember: vi.fn(),
    setMemberRole: vi.fn(),
    listInvites: vi.fn(),
    revokeInvite: vi.fn(),
    issueToken: vi.fn(),
    listTokens: vi.fn(),
    revokeToken: vi.fn(),
  }
})

const mocked = vi.mocked(api)

function makeMember(overrides: Partial<Member> = {}): Member {
  return {
    user_id: 'u1',
    email: 'owner@example.com',
    display_name: 'Owner',
    role: 'owner',
    joined_at: '2026-09-01T00:00:00Z',
    ...overrides,
  }
}

function makeInvite(overrides: Partial<Invite> = {}): Invite {
  return {
    id: 'inv1',
    workspace_id: 'ws1',
    email: 'ghost@example.com',
    role: 'member',
    code: 'abcd1234',
    status: 'pending',
    invited_by: 'u1',
    created_at: '2026-09-04T00:00:00Z',
    ...overrides,
  }
}

function makeToken(overrides: Partial<APIToken> = {}): APIToken {
  return {
    id: 't1',
    name: 'ci',
    prefix: 'spat_abc1234',
    created_at: '2026-09-04T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/settings']}>
      <SettingsPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.listMembers.mockResolvedValue({
    workspace: { id: 'ws1', name: 'Acme' },
    viewer_role: 'owner',
    members: [makeMember(), makeMember({ user_id: 'u2', email: 'mate@example.com', display_name: 'Mate', role: 'member' })],
  })
  mocked.listInvites.mockResolvedValue({ invites: [] })
  mocked.listTokens.mockResolvedValue({ tokens: [] })
})

describe('SettingsPage', () => {
  it('lists workspace members with roles', async () => {
    renderPage()

    expect(await screen.findByTestId('member-u1')).toBeInTheDocument()
    expect(screen.getByTestId('member-u2')).toBeInTheDocument()
    expect(screen.getByTestId('workspace-name')).toHaveTextContent('Acme')
  })

  it('hides role editing for non-owner viewers', async () => {
    mocked.listMembers.mockResolvedValue({
      workspace: { id: 'ws1', name: 'Acme' },
      viewer_role: 'member',
      members: [makeMember()],
    })
    renderPage()

    expect(await screen.findByTestId('member-u1')).toBeInTheDocument()
    expect(screen.queryByTestId('role-select-u1')).not.toBeInTheDocument()
    expect(screen.queryByTestId('invite-form')).not.toBeInTheDocument()
  })

  it('changes a member role from the select', async () => {
    const user = userEvent.setup()
    mocked.setMemberRole.mockResolvedValue({ member: makeMember() })
    renderPage()

    const select = await screen.findByTestId('role-select-u2')
    await user.selectOptions(select, 'owner')

    await waitFor(() => {
      expect(mocked.setMemberRole).toHaveBeenCalledWith('u2', 'owner')
    })
  })

  it('invites a registered user and reports direct join', async () => {
    const user = userEvent.setup()
    mocked.inviteMember.mockResolvedValue({ joined: true, member: makeMember() })
    renderPage()

    await user.type(await screen.findByTestId('invite-email'), 'new@example.com')
    await user.click(screen.getByTestId('invite-submit'))

    expect(mocked.inviteMember).toHaveBeenCalledWith('new@example.com', 'member')
    expect(await screen.findByTestId('invite-joined')).toBeInTheDocument()
  })

  it('invites an unknown email and shows the code once', async () => {
    const user = userEvent.setup()
    mocked.inviteMember.mockResolvedValue({ joined: false, code: 'code9999' })
    renderPage()

    await user.type(await screen.findByTestId('invite-email'), 'ghost@example.com')
    await user.click(screen.getByTestId('invite-submit'))

    expect(mocked.inviteMember).toHaveBeenCalledWith('ghost@example.com', 'member')
    const code = await screen.findByTestId('invite-code')
    expect(code).toHaveTextContent('code9999')
  })

  it('lists pending invites and revokes one', async () => {
    const user = userEvent.setup()
    mocked.listInvites.mockResolvedValue({ invites: [makeInvite()] })
    mocked.revokeInvite.mockResolvedValue({ invite: makeInvite({ status: 'revoked' }) })
    renderPage()

    expect(await screen.findByTestId('invite-inv1')).toBeInTheDocument()
    await user.click(screen.getByTestId('revoke-invite-inv1'))

    await waitFor(() => {
      expect(mocked.revokeInvite).toHaveBeenCalledWith('inv1')
    })
  })

  it('issues a token and shows the plaintext only once', async () => {
    const user = userEvent.setup()
    mocked.issueToken.mockResolvedValue({
      token: 'spat_secret_secret_secret',
      token_record: makeToken(),
    })
    renderPage()

    await user.type(await screen.findByTestId('token-name'), 'ci')
    await user.click(screen.getByTestId('token-submit'))

    expect(mocked.issueToken).toHaveBeenCalledWith('ci')
    expect(await screen.findByTestId('issued-token')).toHaveTextContent('spat_secret_secret_secret')

    // Dismissing hides the plaintext (it is never shown again).
    await user.click(screen.getByTestId('dismiss-issued-token'))
    expect(screen.queryByTestId('issued-token')).not.toBeInTheDocument()
  })

  it('lists tokens with revoked state and revokes one', async () => {
    const user = userEvent.setup()
    mocked.listTokens.mockResolvedValue({
      tokens: [makeToken(), makeToken({ id: 't2', name: 'old', revoked_at: '2026-09-05T00:00:00Z' })],
    })
    mocked.revokeToken.mockResolvedValue(undefined)
    renderPage()

    expect(await screen.findByTestId('token-t1')).toHaveAttribute('data-revoked', 'false')
    expect(screen.getByTestId('token-t2')).toHaveAttribute('data-revoked', 'true')

    await user.click(screen.getByTestId('revoke-token-t1'))
    await waitFor(() => {
      expect(mocked.revokeToken).toHaveBeenCalledWith('t1')
    })
  })
})
