import { Navigate, Route, Routes } from 'react-router'

import { accountSupport, adminOnly, caseworkers, customersOnly, homeFor } from './auth/access'
import { useAuth } from './auth/AuthProvider'
import { RequireRole } from './auth/RequireRole'
import { Loading } from './components/Feedback'
import { Layout } from './components/Layout'
import { AuditLogPage } from './features/admin/AuditLogPage'
import { NotificationsPage } from './features/admin/NotificationsPage'
import { OverviewPage } from './features/admin/OverviewPage'
import { LoginPage } from './features/auth/LoginPage'
import { DisputeDetailPage } from './features/disputes/DisputeDetailPage'
import { MyDisputesPage } from './features/disputes/MyDisputesPage'
import { LockedAccountsPage } from './features/staff/LockedAccountsPage'
import { QueuePage } from './features/staff/QueuePage'
import { NotFoundPage } from './features/system/SystemPages'
import { TransactionsPage } from './features/transactions/TransactionsPage'

function Home() {
  const { status, user } = useAuth()
  if (status === 'loading') return <Loading label="Checking your session" />
  return <Navigate to={user ? homeFor(user.role) : '/login'} replace />
}

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<Layout />}>
        <Route index element={<Home />} />

        <Route path="transactions" element={<RequireRole roles={customersOnly}><TransactionsPage /></RequireRole>} />
        <Route path="disputes" element={<RequireRole roles={customersOnly}><MyDisputesPage /></RequireRole>} />
        <Route path="disputes/:id" element={<RequireRole roles={customersOnly}><DisputeDetailPage /></RequireRole>} />

        <Route path="staff/disputes" element={<RequireRole roles={caseworkers}><QueuePage /></RequireRole>} />
        <Route path="staff/disputes/:id" element={<RequireRole roles={caseworkers}><DisputeDetailPage /></RequireRole>} />
        <Route path="staff/accounts" element={<RequireRole roles={accountSupport}><LockedAccountsPage /></RequireRole>} />

        <Route path="admin" element={<RequireRole roles={adminOnly}><OverviewPage /></RequireRole>} />
        <Route path="admin/audit" element={<RequireRole roles={adminOnly}><AuditLogPage /></RequireRole>} />
        <Route path="admin/notifications" element={<RequireRole roles={adminOnly}><NotificationsPage /></RequireRole>} />

        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
