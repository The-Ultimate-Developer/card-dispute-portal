import { http, HttpResponse } from 'msw'

import type { DisputeDetail, Problem, Role, Session, Transaction } from '../api/types'

const users: Record<Role, Session['user']> = {
  CUSTOMER: { id: '7d1c2f0e-0000-4000-8000-000000000001', email: 'thandi@demo.capitec.local', fullName: 'Thandi Mokoena', role: 'CUSTOMER' },
  AGENT: { id: '7d1c2f0e-0000-4000-8000-000000000002', email: 'agent@demo.capitec.local', fullName: 'Lerato Agent', role: 'AGENT' },
  SUPPORT: { id: '7d1c2f0e-0000-4000-8000-000000000003', email: 'support@demo.capitec.local', fullName: 'Sam Support', role: 'SUPPORT' },
  ADMIN: { id: '7d1c2f0e-0000-4000-8000-000000000004', email: 'admin@demo.capitec.local', fullName: 'Ayanda Admin', role: 'ADMIN' },
}

export const CSRF_TOKEN = 'csrf-test-token'

export function session(role: Role): Session {
  return { user: users[role], csrfToken: CSRF_TOKEN }
}

export function problem(status: number, code: string, detail: string, extra: Partial<Problem> = {}) {
  return HttpResponse.json(
    { type: 'about:blank', title: detail, status, detail, code, requestId: 'req-0123456789abcdef', ...extra },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

export const signedInAs = (role: Role) => http.get('/api/auth/me', () => HttpResponse.json(session(role)))
export const signedOut = http.get('/api/auth/me', () => problem(401, 'UNAUTHENTICATED', 'Sign in to continue.'))

export function transaction(overrides: Partial<Transaction> = {}): Transaction {
  return {
    id: 'a3f1c9de-0000-4000-8000-000000000101',
    cardId: 'c0000000-0000-4000-8000-000000000001',
    cardBrand: 'VISA',
    cardLast4: '4821',
    merchant: 'Takealot',
    category: 'Online shopping',
    amountCents: 129_999,
    currency: 'ZAR',
    status: 'POSTED',
    occurredAt: '2026-10-20T09:15:00Z',
    disputable: true,
    dispute: null,
    ...overrides,
  }
}

export function dispute(overrides: Partial<DisputeDetail> = {}): DisputeDetail {
  return {
    id: 'd15e0a7b-0000-4000-8000-000000000201',
    transactionId: 'a3f1c9de-0000-4000-8000-000000000101',
    merchant: 'Takealot',
    amountCents: 129_999,
    currency: 'ZAR',
    cardLast4: '4821',
    transactionOccurredAt: '2026-10-20T09:15:00Z',
    reason: 'NOT_RECEIVED',
    description: 'The parcel never arrived and the tracking stopped.',
    status: 'SUBMITTED',
    version: 1,
    createdAt: '2026-10-21T08:00:00Z',
    updatedAt: '2026-10-21T08:00:00Z',
    timeline: [
      {
        id: 1,
        fromStatus: null,
        toStatus: 'SUBMITTED',
        actorRole: 'CUSTOMER',
        note: 'The parcel never arrived and the tracking stopped.',
        createdAt: '2026-10-21T08:00:00Z',
      },
    ],
    ...overrides,
  }
}
