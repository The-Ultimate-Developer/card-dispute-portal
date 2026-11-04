import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { ChangeStatusRequest } from '../../api/types'
import { dispute, problem, signedInAs } from '../../test/fixtures'
import { renderApp } from '../../test/render'
import { server } from '../../test/server'

const customer = { id: 'c1', fullName: 'Thandi Mokoena', email: 'thandi@demo.capitec.local' }

describe('agent status actions', () => {
  it('sends the version it read and shows the new status', async () => {
    let sent: ChangeStatusRequest | undefined
    server.use(
      signedInAs('AGENT'),
      http.get('/api/disputes/:id', () => HttpResponse.json(dispute({ customer }))),
      http.get('/api/disputes', () => HttpResponse.json({ items: [] })),
      http.patch('/api/disputes/:id/status', async ({ request }) => {
        sent = (await request.json()) as ChangeStatusRequest
        return HttpResponse.json(
          dispute({
            customer,
            status: 'UNDER_REVIEW',
            version: 2,
            timeline: [
              ...dispute().timeline,
              { id: 2, fromStatus: 'SUBMITTED', toStatus: 'UNDER_REVIEW', actorRole: 'AGENT', note: '', createdAt: '2026-10-22T08:00:00Z' },
            ],
          }),
        )
      }),
    )
    const { user } = renderApp(`/staff/disputes/${dispute().id}`)

    expect(await screen.findByText('thandi@demo.capitec.local')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Start review' }))
    await user.click(screen.getByRole('button', { name: 'Confirm: start review' }))

    expect(await screen.findByText(/Dispute moved to under review/)).toBeInTheDocument()
    expect(sent).toEqual({ status: 'UNDER_REVIEW', version: 1, note: '' })
    expect(screen.getByRole('button', { name: 'Resolve in favour of customer' })).toBeInTheDocument()
  })

  it('requires a note before closing a dispute', async () => {
    server.use(signedInAs('AGENT'), http.get('/api/disputes/:id', () => HttpResponse.json(dispute({ customer }))))
    const { user } = renderApp(`/staff/disputes/${dispute().id}`)

    await user.click(await screen.findByRole('button', { name: 'Decline dispute' }))
    await user.click(screen.getByRole('button', { name: 'Confirm: decline dispute' }))

    expect(screen.getByText('Add a note for the customer explaining the outcome')).toBeInTheDocument()
  })

  it('reloads the dispute when someone else changed it first', async () => {
    let reads = 0
    server.use(
      signedInAs('AGENT'),
      http.get('/api/disputes/:id', () => {
        reads += 1
        return HttpResponse.json(reads === 1 ? dispute({ customer }) : dispute({ customer, status: 'REJECTED', version: 2 }))
      }),
      http.patch('/api/disputes/:id/status', () => problem(409, 'STALE_VERSION', 'Changed by someone else')),
    )
    const { user } = renderApp(`/staff/disputes/${dispute().id}`)

    await user.click(await screen.findByRole('button', { name: 'Start review' }))
    await user.click(screen.getByRole('button', { name: 'Confirm: start review' }))

    expect(await screen.findByText('This dispute is closed. No further actions are available.')).toBeInTheDocument()
    expect(reads).toBe(2)
  })
})
