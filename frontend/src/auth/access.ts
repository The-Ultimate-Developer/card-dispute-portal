import type { Role } from '../api/types'

export interface NavItem {
  to: string
  label: string
}

// One place defines what each role sees. The API enforces the same policy;
// this only shapes navigation so nobody is offered a page that would 403.
export const navigation: Record<Role, NavItem[]> = {
  CUSTOMER: [
    { to: '/transactions', label: 'Transactions' },
    { to: '/disputes', label: 'My disputes' },
  ],
  AGENT: [{ to: '/staff/disputes', label: 'Dispute queue' }],
  SUPPORT: [{ to: '/staff/accounts', label: 'Locked accounts' }],
  ADMIN: [
    { to: '/admin', label: 'Overview' },
    { to: '/staff/disputes', label: 'Dispute queue' },
    { to: '/staff/accounts', label: 'Locked accounts' },
    { to: '/admin/audit', label: 'Sign-in activity' },
    { to: '/admin/notifications', label: 'Emails' },
  ],
}

export function homeFor(role: Role): string {
  return navigation[role][0]?.to ?? '/'
}

export const roleLabels: Record<Role, string> = {
  CUSTOMER: 'Customer',
  AGENT: 'Disputes agent',
  SUPPORT: 'Support',
  ADMIN: 'Administrator',
}

export const caseworkers: Role[] = ['AGENT', 'ADMIN']
export const accountSupport: Role[] = ['SUPPORT', 'ADMIN']
export const adminOnly: Role[] = ['ADMIN']
export const customersOnly: Role[] = ['CUSTOMER']
