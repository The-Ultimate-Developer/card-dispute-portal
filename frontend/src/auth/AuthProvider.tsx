import { type QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

import { isApiError, onSessionExpired, request, setCsrfToken } from '../api/client'
import type { Session, User } from '../api/types'

type AuthStatus = 'loading' | 'authenticated' | 'anonymous'

interface AuthContextValue {
  status: AuthStatus
  user: User | null
  sessionExpired: boolean
  login: (email: string, password: string) => Promise<User>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)
const SESSION_KEY = ['session'] as const

// The session cookie is httpOnly, so the app can't read it. It asks the API who
// is signed in (/api/auth/me) and keeps the answer, plus the CSRF token, in memory.
async function fetchSession(): Promise<Session | null> {
  try {
    return await request<Session>('/api/auth/me')
  } catch (error) {
    if (isApiError(error) && error.status === 401) return null
    throw error
  }
}

// Drops the CSRF token and every cached response, but keeps the session query
// itself so the components watching it see it become null and redirect.
function endSession(queryClient: QueryClient) {
  setCsrfToken(null)
  queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== SESSION_KEY[0] })
  queryClient.setQueryData(SESSION_KEY, null)
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [sessionExpired, setSessionExpired] = useState(false)

  const session = useQuery({
    queryKey: SESSION_KEY,
    queryFn: fetchSession,
    staleTime: Infinity,
    retry: false,
  })

  useEffect(() => {
    setCsrfToken(session.data?.csrfToken ?? null)
  }, [session.data])

  useEffect(
    () =>
      onSessionExpired(() => {
        // The first /me probe of a signed-out visitor is also a 401; only a session we had can expire.
        if (!queryClient.getQueryData(SESSION_KEY)) return
        endSession(queryClient)
        setSessionExpired(true)
      }),
    [queryClient],
  )

  const loginMutation = useMutation({
    mutationFn: (credentials: { email: string; password: string }) =>
      request<Session>('/api/auth/login', { method: 'POST', body: credentials }),
  })

  const login = useCallback(
    async (email: string, password: string) => {
      const next = await loginMutation.mutateAsync({ email, password })
      setCsrfToken(next.csrfToken)
      setSessionExpired(false)
      queryClient.setQueryData(SESSION_KEY, next)
      return next.user
    },
    [loginMutation, queryClient],
  )

  const logout = useCallback(async () => {
    try {
      await request<void>('/api/auth/logout', { method: 'POST' })
    } finally {
      endSession(queryClient)
    }
  }, [queryClient])

  const value = useMemo<AuthContextValue>(() => {
    let status: AuthStatus = 'anonymous'
    if (session.isPending) status = 'loading'
    else if (session.data) status = 'authenticated'
    return { status, user: session.data?.user ?? null, sessionExpired, login, logout }
  }, [session.isPending, session.data, sessionExpired, login, logout])

  return <AuthContext value={value}>{children}</AuthContext>
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used inside <AuthProvider>')
  return context
}
