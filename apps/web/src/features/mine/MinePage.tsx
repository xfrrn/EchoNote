import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { useAuth } from '../auth/AuthProvider'
import { useThemeStore } from '../../shared/store/theme'
import { useCaptureOutbox } from '../../shared/outbox/useCaptureOutbox'
import { SegmentedControl } from '../../shared/components/SegmentedControl'
import { SectionLabel } from '../../shared/components/SectionLabel'
import { EchoMark } from '../../shared/components/EchoMark'

export function MinePage() {
  const { session, logout } = useAuth()
  const theme = useThemeStore((state) => state.theme)
  const setTheme = useThemeStore((state) => state.setTheme)
  const outbox = useCaptureOutbox()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const signOut = async (clearPending: boolean) => {
    setBusy(true); setError('')
    try { await logout(clearPending) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '退出登录失败。') }
    finally { setBusy(false) }
  }

  return (
    <div>
      <header className="px-4 pt-4"><h1 className="text-large-title text-ink">我的</h1></header>
      <div className="mx-4 mt-4 flex items-center gap-3.5 rounded-lg bg-surface px-4 py-4"><span className="flex h-12 w-12 items-center justify-center rounded-lg bg-accent-soft text-accent"><EchoMark size={28} /></span><div className="min-w-0"><p className="text-headline text-ink">{session?.user.username}</p><p className="mt-0.5 text-caption text-ink-secondary">会话有效期至 {session ? new Date(session.expires_at).toLocaleString() : '—'}</p></div></div>

      <SectionLabel>外观</SectionLabel>
      <div className="px-4 py-4"><SegmentedControl ariaLabel="外观模式" value={theme} onChange={setTheme} options={[{ value: 'light', label: '浅色' }, { value: 'dark', label: '深色' }, { value: 'system', label: '系统' }]} /></div>

      <SectionLabel>离线记录</SectionLabel>
      <div className="px-4 py-4"><p className="text-body text-ink">{outbox.length ? `${outbox.length} 条待发送` : '已全部同步'}</p><p className="mt-1 text-caption text-ink-tertiary">{outbox.some((item) => item.state === 'blocked') ? '有记录需要回到对应节目手动重试或删除。' : '退出登录默认保留本地待发送记录。'}</p></div>

      <SectionLabel>账号</SectionLabel>
      <div className="space-y-2 px-4 py-4">
        <button type="button" disabled={busy} onClick={() => void signOut(false)} className="min-h-11 w-full rounded-md bg-subtle text-callout text-ink disabled:opacity-40">退出并保留待发送记录</button>
        <button type="button" disabled={busy || outbox.length === 0} onClick={() => { if (window.confirm(`退出并永久清除 ${outbox.length} 条本地待发送记录？`)) void signOut(true) }} className="min-h-11 w-full rounded-md text-callout text-danger disabled:opacity-40">退出并清除待发送记录</button>
        {error ? <p role="alert" className="text-callout text-danger">{error}</p> : null}
      </div>

      {import.meta.env.DEV ? <><SectionLabel>开发</SectionLabel><Link to="/dev/design" className="flex min-h-11 items-center justify-between px-4 py-3 active:bg-subtle"><span className="text-body text-ink">Design Playground</span><span className="flex items-center gap-1 text-caption text-ink-tertiary">/dev/design<ChevronRight size={18} aria-hidden /></span></Link></> : null}
      <p className="px-4 py-6 text-caption text-ink-tertiary">EchoNote v0.1 · API 数据已启用</p>
    </div>
  )
}
