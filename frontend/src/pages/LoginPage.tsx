import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { ApiError } from '../api/client'

export function LoginPage() {
  const { login, register } = useAuth()
  const navigate = useNavigate()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      if (mode === 'login') {
        await login(email, password)
      } else {
        await register(email, password, displayName)
      }
      navigate('/')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '请求失败，请稍后重试')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={onSubmit}>
        <h1>{mode === 'login' ? '登录' : '注册'}</h1>
        {mode === 'register' && (
          <label>
            显示名
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              data-testid="display-name"
              required
            />
          </label>
        )}
        <label>
          邮箱
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            data-testid="email"
            required
          />
        </label>
        <label>
          密码
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            data-testid="password"
            required
          />
        </label>
        {error && (
          <p role="alert" className="login-error" data-testid="error">
            {error}
          </p>
        )}
        <button type="submit" disabled={busy} data-testid="submit">
          {busy ? '提交中…' : mode === 'login' ? '登录' : '注册'}
        </button>
        <button
          type="button"
          className="linklike"
          onClick={() => {
            setMode(mode === 'login' ? 'register' : 'login')
            setError('')
          }}
        >
          {mode === 'login' ? '没有账号？去注册' : '已有账号？去登录'}
        </button>
      </form>
    </div>
  )
}
