import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { EchoMark } from '../../shared/components/EchoMark'
import { ApiError } from '../../shared/api/client'
import { useAuth } from './AuthProvider'

export function LoginPage() {
  const { session, login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (session) return <Navigate to="/library" replace />

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!username.trim() || !password) return
    setBusy(true)
    setError('')
    try {
      await login({ username: username.trim(), password })
      const target = (location.state as { from?: string } | null)?.from ?? '/library'
      navigate(target, { replace: true })
    } catch (reason) {
      setError(reason instanceof ApiError && reason.status === 401 ? '用户名或密码不正确。' : reason instanceof Error ? reason.message : '登录失败，请稍后重试。')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="safe-top safe-bottom safe-sides app-viewport flex items-center justify-center bg-canvas px-4 text-ink">
      <form onSubmit={submit} className="w-full max-w-sm rounded-lg bg-surface p-5 shadow-control">
        <div className="flex items-center gap-3 text-accent">
          <EchoMark size={32} />
          <div>
            <h1 className="text-title-2 text-ink">登录 EchoNote</h1>
            <p className="mt-0.5 text-caption text-ink-secondary">你的节目、笔记与会话只对当前账号可见。</p>
          </div>
        </div>
        <label className="mt-6 block text-caption-medium text-ink-secondary">
          用户名
          <input
            autoFocus
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            className="mt-1.5 min-h-12 w-full rounded-md border border-hairline bg-canvas px-3 text-body text-ink focus:border-accent focus:outline-none"
          />
        </label>
        <label className="mt-4 block text-caption-medium text-ink-secondary">
          密码
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="mt-1.5 min-h-12 w-full rounded-md border border-hairline bg-canvas px-3 text-body text-ink focus:border-accent focus:outline-none"
          />
        </label>
        {error ? <p role="alert" className="mt-3 text-callout text-danger">{error}</p> : null}
        <button
          type="submit"
          disabled={busy || !username.trim() || !password}
          className="mt-5 min-h-12 w-full rounded-md bg-accent text-body font-medium text-on-accent disabled:opacity-40"
        >
          {busy ? '正在登录…' : '登录'}
        </button>
      </form>
    </main>
  )
}
