import { createContext, useContext, useEffect, useState, type PropsWithChildren } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { api, ApiError, UNAUTHORIZED_EVENT, type LoginRequest, type LoginResponse } from '../../shared/api/client'
import { clearCaptureOutbox, replayCaptureOutbox } from '../../shared/outbox/captureOutbox'
import { useCaptureStore } from '../../shared/store/capture'

type AuthContextValue = {
  session: LoginResponse | null | undefined
  login: (credentials: LoginRequest) => Promise<void>
  logout: (clearPending: boolean) => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)
const SESSION_KEY = 'echonote.session'

function cachedSession(): LoginResponse | undefined {
  try {
    const value = sessionStorage.getItem(SESSION_KEY)
    return value ? JSON.parse(value) as LoginResponse : undefined
  } catch {
    return undefined
  }
}

function clearLocalUserState(): void {
  sessionStorage.removeItem(SESSION_KEY)
  localStorage.removeItem('echonote.capture')
  useCaptureStore.setState({ episodeId: '', draft: '' })
  for (let index = localStorage.length - 1; index >= 0; index -= 1) {
    const key = localStorage.key(index)
    if (key?.startsWith('echonote.conversation.')) localStorage.removeItem(key)
  }
}

export function AuthProvider({ children }: PropsWithChildren) {
  const queryClient = useQueryClient()
  const [session, setSession] = useState<LoginResponse | null | undefined>(cachedSession)

  useEffect(() => {
    const unauthorized = () => {
      clearLocalUserState()
      queryClient.clear()
      setSession(null)
    }
    window.addEventListener(UNAUTHORIZED_EVENT, unauthorized)
    const cached = cachedSession()
    api.me().then((value) => {
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(value))
      setSession(value)
    }).catch((error) => {
      if (error instanceof ApiError && error.status === 401) unauthorized()
      else if (!cached) setSession(null)
    })
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, unauthorized)
  }, [queryClient])

  const login = async (credentials: LoginRequest) => {
    const next = await api.login(credentials)
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(next))
    queryClient.clear()
    setSession(next)
    void replayCaptureOutbox()
  }

  const logout = async (clearPending: boolean) => {
    await api.logout()
    if (clearPending) await clearCaptureOutbox()
    clearLocalUserState()
    queryClient.clear()
    setSession(null)
  }

  return <AuthContext.Provider value={{ session, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}

export function RequireAuth({ children }: PropsWithChildren) {
  const { session } = useAuth()
  const location = useLocation()
  if (session === undefined) return <p className="safe-top px-4 py-10 text-center text-body text-ink-secondary">正在验证登录状态…</p>
  if (!session) return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />
  return children
}
