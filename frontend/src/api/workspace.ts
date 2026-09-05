import { apiFetch } from './client'

export interface Member {
  user_id: string
  email: string
  display_name: string
  role: string
  joined_at: string
}

export interface Invite {
  id: string
  workspace_id: string
  email: string
  role: string
  code: string
  status: string
  invited_by: string
  created_at: string
  accepted_at?: string
}

export interface APIToken {
  id: string
  name: string
  prefix: string
  created_at: string
  last_used_at?: string
  revoked_at?: string
}

export function listMembers(): Promise<{ workspace: { id: string; name: string }; viewer_role: string; members: Member[] }> {
  return apiFetch('/workspace/members')
}

export function inviteMember(
  email: string,
  role: string,
): Promise<{ joined: boolean; member?: Member; invite?: Invite; code?: string }> {
  return apiFetch('/workspace/members/invite', { method: 'POST', body: { email, role } })
}

export function setMemberRole(userId: string, role: string): Promise<{ member: Member }> {
  return apiFetch(`/workspace/members/${userId}`, { method: 'PATCH', body: { role } })
}

export function listInvites(): Promise<{ invites: Invite[] }> {
  return apiFetch('/workspace/invites')
}

export function revokeInvite(id: string): Promise<{ invite: Invite }> {
  return apiFetch(`/workspace/invites/${id}`, { method: 'DELETE' })
}

export function issueToken(name: string): Promise<{ token: string; token_record: APIToken }> {
  return apiFetch('/auth/tokens', { method: 'POST', body: { name } })
}

export function listTokens(): Promise<{ tokens: APIToken[] }> {
  return apiFetch('/auth/tokens')
}

export function revokeToken(id: string): Promise<void> {
  return apiFetch(`/auth/tokens/${id}`, { method: 'DELETE' })
}
