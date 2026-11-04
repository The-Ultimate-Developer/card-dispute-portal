import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { CreateDisputeRequest } from '../../api/types'
import { CSRF_TOKEN, dispute, problem, signedInAs, transaction } from '../../test/fixtures'
import { renderApp } from '../../test/render'
import { server } from '../../test/server'

describe('transactions', () => {
  it('shows the right action for each payment', async () => {
    server.use(
      signedInAs('CUSTOMER'),
      http.get('/api/transactions', () =>
        HttpResponse.json({
          items: [
            transaction(),
            transaction({ id: 't2', merchant: 'Uber', disputable: false, dispute: { id: 'd2', status: 'UNDER_REVIEW' } }),
            transaction({ id: 't3', merchant: 'Checkers', disputable: false, status: 'PENDING' }),
            transaction({ id: 't4', merchant: 'Engen', disputable: true, dispute: { id: 'd4', status: 'RESOLVED' } }),
          ],
        }),
      ),
    )
    renderApp('/transactions')

    const rows = await screen.findAllByRole('listitem')
    expect(within(rows[0]!).getByRole('button', { name: /Dispute payment to Takealot/ })).toBeInTheDocument()
    expect(rows[0]!.querySelector('.statement__amount')?.textContent).toBe('R1\u2009299.99')
    expect(within(rows[1]!).getByRole('link', { name: /View dispute for Uber/ })).toHaveAttribute('href', '/disputes/d2')
    expect(within(rows[2]!).getByText('Pending')).toBeInTheDocument()
    expect(within(rows[3]!).getByRole('link', { name: /View dispute for Engen/ })).toHaveTextContent('Resolved')
    expect(within(rows[3]!).getByRole('button', { name: /Dispute payment to Engen/ })).toBeInTheDocument()
  })

  it('submits a dispute once, with CSRF and idempotency headers, and opens it', async () => {
    const keys: (string | null)[] = []
    let body: CreateDisputeRequest | undefined
    server.use(
      signedInAs('CUSTOMER'),
      http.get('/api/transactions', () => HttpResponse.json({ items: [transaction()] })),
      http.post('/api/disputes', async ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe(CSRF_TOKEN)
        keys.push(request.headers.get('Idempotency-Key'))
        body = (await request.json()) as CreateDisputeRequest
        return HttpResponse.json(dispute(), { status: 201 })
      }),
      http.get('/api/disputes/:id', () => HttpResponse.json(dispute())),
    )
    const { user } = renderApp('/transactions')

    await user.click(await screen.findByRole('button', { name: /Dispute payment to Takealot/ }))
    const dialog = screen.getByRole('dialog', { name: 'Dispute this payment' })
    await user.click(within(dialog).getByLabelText("I didn't receive what I paid for"))
    await user.type(within(dialog).getByLabelText('Tell us what happened'), '  The parcel never arrived.  ')
    await user.click(within(dialog).getByRole('button', { name: 'Submit dispute' }))

    expect(await screen.findByText(/Dispute submitted/)).toHaveTextContent('reference D15E0A7B')
    expect(screen.getByRole('list', { name: 'Dispute progress' })).toBeInTheDocument()
    expect(keys).toHaveLength(1)
    expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/)
    expect(body).toEqual({ transactionId: transaction().id, reason: 'NOT_RECEIVED', description: 'The parcel never arrived.' })
  })

  it('keeps the customer on the form when the API rejects a field', async () => {
    server.use(
      signedInAs('CUSTOMER'),
      http.get('/api/transactions', () => HttpResponse.json({ items: [transaction()] })),
      http.post('/api/disputes', () =>
        problem(422, 'VALIDATION_FAILED', 'Check the form', { errors: { description: 'Remove card numbers from the description' } }),
      ),
    )
    const { user } = renderApp('/transactions')

    await user.click(await screen.findByRole('button', { name: /Dispute payment to Takealot/ }))
    await user.click(screen.getByLabelText('Something else'))
    await user.type(screen.getByLabelText('Tell us what happened'), 'My card 4111 was used')
    await user.click(screen.getByRole('button', { name: 'Submit dispute' }))

    expect(await screen.findByText('Remove card numbers from the description')).toBeInTheDocument()
  })

  it('asks for a reason and enough detail before submitting', async () => {
    server.use(signedInAs('CUSTOMER'), http.get('/api/transactions', () => HttpResponse.json({ items: [transaction()] })))
    const { user } = renderApp('/transactions')

    await user.click(await screen.findByRole('button', { name: /Dispute payment to Takealot/ }))
    await user.type(screen.getByLabelText('Tell us what happened'), 'short')
    await user.click(screen.getByRole('button', { name: 'Submit dispute' }))

    expect(screen.getByText('Choose what went wrong')).toBeInTheDocument()
    expect(screen.getByText('Tell us a little more (at least 10 characters)')).toBeInTheDocument()
  })
})

describe('dispute detail for customers', () => {
  it('shows the case track with the steps still to come', async () => {
    server.use(signedInAs('CUSTOMER'), http.get('/api/disputes/:id', () => HttpResponse.json(dispute())))
    renderApp(`/disputes/${dispute().id}`)

    const track = await screen.findByRole('list', { name: 'Dispute progress' })
    const steps = within(track).getAllByRole('listitem')
    expect(steps.map((s) => s.querySelector('.track__title')?.textContent)).toEqual([
      'Submitted',
      'Under review (not yet reached)',
      'Outcome (not yet reached)',
    ])
    expect(steps[0]).toHaveAttribute('aria-current', 'step')
    expect(screen.queryByRole('group', { name: 'Change status' })).not.toBeInTheDocument()
  })

  it('shows not found for a dispute that belongs to someone else', async () => {
    server.use(signedInAs('CUSTOMER'), http.get('/api/disputes/:id', () => problem(404, 'NOT_FOUND', 'Not found')))
    renderApp('/disputes/someone-elses')

    expect(await screen.findByRole('heading', { name: "We couldn't find that dispute" })).toBeInTheDocument()
  })
})
