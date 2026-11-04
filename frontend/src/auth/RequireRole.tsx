import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'

import type { Role } from '../api/types'
import { Loading } from '../components/Feedback'
import { ForbiddenPage } from '../features/system/SystemPages'
import { useAuth } from './AuthProvider'

// UX only: it hides pages a role can't use. Every API call is still authorised
// on the server, so bypassing this in the browser gains nothing.
export function RequireRole({ roles, children }: { roles: Role[]; children: ReactNode }) {
  const { status, user } = useAuth()
  const location = useLocation()

  if (status === 'loading') return <Loading label="Checking your session" />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  if (!roles.includes(user.role)) return <ForbiddenPage />
  return children
}
