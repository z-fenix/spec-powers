import { useCallback, useEffect, useState } from 'react'
import {
  inviteMember,
  issueToken,
  listInvites,
  listMembers,
  listTokens,
  revokeInvite,
  revokeToken,
  setMemberRole,
  type Invite,
  type Member,
  type APIToken,
} from '../api/workspace'

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function SettingsPage() {
  const [workspaceName, setWorkspaceName] = useState('')
  const [viewerRole, setViewerRole] = useState('member')
  const [members, setMembers] = useState<Member[]>([])
  const [invites, setInvites] = useState<Invite[]>([])
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [loaded, setLoaded] = useState(false)

  // invite form
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState('member')
  const [inviteResult, setInviteResult] = useState<{ joined: boolean; code?: string } | null>(null)

  // token form
  const [tokenName, setTokenName] = useState('')
  const [issuedToken, setIssuedToken] = useState<string | null>(null)

  const refreshMembers = useCallback(() => {
    return listMembers()
      .then((res) => {
        setWorkspaceName(res.workspace.name)
        setViewerRole(res.viewer_role)
        setMembers(res.members)
      })
      .catch(() => setMembers([]))
  }, [])

  const refreshInvites = useCallback(() => {
    return listInvites()
      .then((res) => setInvites(res.invites))
      .catch(() => setInvites([]))
  }, [])

  const refreshTokens = useCallback(() => {
    return listTokens()
      .then((res) => setTokens(res.tokens))
      .catch(() => setTokens([]))
  }, [])

  useEffect(() => {
    Promise.all([refreshMembers(), refreshInvites(), refreshTokens()]).then(() => setLoaded(true))
  }, [refreshMembers, refreshInvites, refreshTokens])

  const isOwner = viewerRole === 'owner'

  const onInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!inviteEmail.trim()) return
    const res = await inviteMember(inviteEmail.trim(), inviteRole)
    setInviteResult({ joined: res.joined, code: res.code })
    setInviteEmail('')
    await Promise.all([refreshMembers(), refreshInvites()])
  }

  const onRoleChange = async (userId: string, role: string) => {
    await setMemberRole(userId, role)
    await refreshMembers()
  }

  const onRevokeInvite = async (id: string) => {
    await revokeInvite(id)
    await refreshInvites()
  }

  const onIssueToken = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!tokenName.trim()) return
    const res = await issueToken(tokenName.trim())
    setIssuedToken(res.token)
    setTokenName('')
    await refreshTokens()
  }

  const onRevokeToken = async (id: string) => {
    await revokeToken(id)
    await refreshTokens()
  }

  const dismissIssued = () => setIssuedToken(null)

  if (!loaded) return <p>加载中…</p>

  return (
    <div className="page" data-testid="settings-page">
      <div className="page-header">
        <h1 className="page-title">工作区设置</h1>
      </div>
      <div className="page-body">

      <h3>
        成员{workspaceName ? ' · ' : ''}
        {workspaceName && (
          <span data-testid="workspace-name">{workspaceName}</span>
        )}
      </h3>
      {members.length === 0 ? (
        <p data-testid="members-empty">暂无成员</p>
      ) : (
        <ul className="member-list" data-testid="member-list">
          {members.map((m) => (
            <li key={m.user_id} data-testid={`member-${m.user_id}`}>
              <span className="member-name">{m.display_name}</span>{' '}
              <span className="member-email">{m.email}</span>{' '}
              {isOwner ? (
                <select
                  aria-label={`调整 ${m.display_name} 的角色`}
                  data-testid={`role-select-${m.user_id}`}
                  value={m.role}
                  onChange={(e) => onRoleChange(m.user_id, e.target.value)}
                >
                  <option value="owner">owner</option>
                  <option value="member">member</option>
                </select>
              ) : (
                <span className="member-role">{m.role}</span>
              )}
            </li>
          ))}
        </ul>
      )}

      {isOwner && (
        <form data-testid="invite-form" onSubmit={onInvite}>
          <h4>邀请成员</h4>
          <input
            type="email"
            placeholder="email@example.com"
            data-testid="invite-email"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
          />
          <select
            aria-label="邀请角色"
            data-testid="invite-role"
            value={inviteRole}
            onChange={(e) => setInviteRole(e.target.value)}
          >
            <option value="member">member</option>
            <option value="owner">owner</option>
          </select>
          <button type="submit" data-testid="invite-submit">
            邀请
          </button>
          {inviteResult && !inviteResult.joined && inviteResult.code && (
            <p data-testid="invite-code">
              邀请码（请复制发给对方，对方登录后凭码加入）：<code>{inviteResult.code}</code>
            </p>
          )}
          {inviteResult && inviteResult.joined && <p data-testid="invite-joined">该用户已直接加入工作区。</p>}
        </form>
      )}

      {isOwner && invites.length > 0 && (
        <>
          <h4>待处理邀请</h4>
          <ul data-testid="invite-list">
            {invites.map((i) => (
              <li key={i.id} data-testid={`invite-${i.id}`}>
                <span>{i.email}</span> <span>({i.role})</span>{' '}
                <code>{i.code}</code>{' '}
                <button data-testid={`revoke-invite-${i.id}`} onClick={() => onRevokeInvite(i.id)}>
                  撤销
                </button>
              </li>
            ))}
          </ul>
        </>
      )}

      <h3>API 令牌</h3>
      {issuedToken && (
        <div data-testid="issued-token-box">
          <p>令牌已签发，请立即复制（仅显示这一次）：</p>
          <code data-testid="issued-token">{issuedToken}</code>
          <button data-testid="dismiss-issued-token" onClick={dismissIssued}>
            我已保存
          </button>
        </div>
      )}
      <form data-testid="token-form" onSubmit={onIssueToken}>
        <input
          type="text"
          placeholder="令牌名称，如 CI"
          data-testid="token-name"
          value={tokenName}
          onChange={(e) => setTokenName(e.target.value)}
        />
        <button type="submit" data-testid="token-submit">
          签发令牌
        </button>
      </form>
      {tokens.length === 0 ? (
        <p data-testid="tokens-empty">暂无令牌</p>
      ) : (
        <ul className="token-list" data-testid="token-list">
          {tokens.map((t) => (
            <li key={t.id} data-testid={`token-${t.id}`} data-revoked={String(Boolean(t.revoked_at))}>
              <span className="token-name">{t.name}</span> <code>{t.prefix}…</code>{' '}
              <time>{formatTime(t.created_at)}</time>{' '}
              {t.revoked_at ? (
                <span>已吊销</span>
              ) : (
                <button data-testid={`revoke-token-${t.id}`} onClick={() => onRevokeToken(t.id)}>
                  吊销
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
    </div>
  )
}
