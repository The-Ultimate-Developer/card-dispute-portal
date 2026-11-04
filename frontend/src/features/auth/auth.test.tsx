import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { problem, session, signedInAs, signedOut, transaction } from '../../test/fixtures'
import { renderApp } from '../../test/render'
import { server } from '../../test/server'

const emptyPage = { items: [] }

describe('signing in', () => {
  it('sends a signed-out visitor to the login page and back where they were going', async () => {
    let signedIn = false
    server.use(
      http.get('/api/auth/me', () => (signedIn ? HttpResponse.json(session('CUSTOMER')) : problem(401, 'UNAUTHENTICATED', 'Sign in'))),
      http.post('/api/auth/login', async ({ request }) => {
        expect(await request.json()).toEqual({ email: 'thandi@demo.capitec.local', password: 'Demo@2026!' })
        signedIn = true
        return HttpResponse.json(session('CUSTOMER'))
      }),
      http.get('/api/disputes', () => HttpResponse.json(emptyPage)),
    )
    const { user } = renderApp('/disputes')

    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
    expect(screen.queryByText(/Your session ended/)).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Email address'), 'thandi@demo.capitec.local')
    await user.type(screen.getByLabelText('Password'), 'Demo@2026!')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByRole('heading', { name: 'My disputes' })).toBeInTheDocument()
  })

  it('validates the form before calling the API', async () => {
    server.use(signedOut)
    const { user } = renderApp('/login')

    await user.click(await screen.findByRole('button', { name: 'Sign in' }))

    expect(screen.getByText('Enter your email address')).toBeInTheDocument()
    expect(screen.getByText('Enter your password')).toBeInTheDocument()
  })

  it.each([
    ['INVALID_CREDENTIALS', 401, 'Incorrect email or password.'],
    ['ACCOUNT_LOCKED', 423, 'Please contact support to unlock it.'],
  ])('explains %s', async (code, status, message) => {
    server.use(
      signedOut,
      http.post('/api/auth/login', () => problem(status, code, 'nope')),
    )
    const { user } = renderApp('/login')

    await user.type(await screen.findByLabelText('Email address'), 'thandi@demo.capitec.local')
    await user.type(screen.getByLabelText('Password'), 'wrong')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(message)
  })

  it('fills the form from a demo account', async () => {
    server.use(signedOut)
    const { user } = renderApp('/login')

    await user.click(await screen.findByRole('button', { name: /Disputes agent/ }))

    expect(screen.getByLabelText('Email address')).toHaveValue('agent@demo.capitec.local')
    expect(screen.getByLabelText('Password')).toHaveValue('Demo@2026!')
  })
})

describe('role guards', () => {
  it('sends each role to its own home page', async () => {
    server.use(signedInAs('SUPPORT'), http.get('/api/support/locked-accounts', () => HttpResponse.json(emptyPage)))
    renderApp('/')

    expect(await screen.findByRole('heading', { name: 'Locked accounts' })).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Main' })
    expect(within(nav).getAllByRole('link').map((l) => l.textContent)).toEqual(['Locked accounts'])
  })

  it('refuses a customer on a staff page without calling the staff API', async () => {
    server.use(signedInAs('CUSTOMER'))
    renderApp('/staff/disputes')

    expect(await screen.findByRole('heading', { name: "You don't have access to this page" })).toBeInTheDocument()
  })
})

describe('session expiry', () => {
  it('returns to the login page with an explanation when the API says the session ended', async () => {
    server.use(
      signedInAs('CUSTOMER'),
      http.get('/api/transactions', () => problem(401, 'UNAUTHENTICATED', 'Sign in to continue.')),
    )
    renderApp('/transactions')

    expect(await screen.findByText('Your session ended. Sign in again to continue.')).toBeInTheDocument()
  })

  it('signs out and clears the session', async () => {
    let loggedOut = false
    server.use(
      http.get('/api/auth/me', () => (loggedOut ? problem(401, 'UNAUTHENTICATED', 'x') : HttpResponse.json(session('CUSTOMER')))),
      http.get('/api/transactions', () => HttpResponse.json({ items: [transaction()] })),
      http.post('/api/auth/logout', ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        loggedOut = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderApp('/transactions')

    await user.click(await screen.findByRole('button', { name: 'Sign out' }))

    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
    expect(loggedOut).toBe(true)
  })
})
