import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { tokenStorage } from '@/api/client'

interface SessionPayload {
  email?: string  // not in the JWT today; we hydrate from login form
  role: string
  expiresAt: number  // unix ms
}

interface AuthState {
  isAuthenticated: boolean
  session: SessionPayload | null
  setSession: (s: SessionPayload, token: string) => void
  logout: () => void
}

const SESSION_KEY = 'sms_session'

const AuthContext = createContext<AuthState | null>(null)

function loadSession(): SessionPayload | null {
  const raw = localStorage.getItem(SESSION_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as SessionPayload
    if (parsed.expiresAt < Date.now()) return null
    return parsed
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<SessionPayload | null>(() => {
    if (!tokenStorage.get()) return null
    return loadSession()
  })

  const setSession = useCallback((s: SessionPayload, token: string) => {
    tokenStorage.set(token)
    localStorage.setItem(SESSION_KEY, JSON.stringify(s))
    setSessionState(s)
  }, [])

  const logout = useCallback(() => {
    tokenStorage.clear()
    localStorage.removeItem(SESSION_KEY)
    setSessionState(null)
  }, [])

  // Auto-logout when token expires while the tab is open.
  useEffect(() => {
    if (!session) return
    const remaining = session.expiresAt - Date.now()
    if (remaining <= 0) {
      logout()
      return
    }
    const t = setTimeout(logout, remaining)
    return () => clearTimeout(t)
  }, [session, logout])

  const value = useMemo<AuthState>(
    () => ({
      isAuthenticated: !!session && !!tokenStorage.get(),
      session,
      setSession,
      logout,
    }),
    [session, setSession, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}
