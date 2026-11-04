import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Navigate, useLocation } from 'react-router'
import { z } from 'zod'

import { isApiError } from '../../api/client'
import { homeFor } from '../../auth/access'
import { useAuth } from '../../auth/AuthProvider'
import { Loading } from '../../components/Feedback'

const schema = z.object({
  email: z.string().trim().min(1, 'Enter your email address').email('Enter a valid email address'),
  password: z.string().min(1, 'Enter your password'),
})

type FormValues = z.infer<typeof schema>

const demoAccounts = [
  { email: 'thandi@demo.capitec.local', role: 'Customer' },
  { email: 'sipho@demo.capitec.local', role: 'Customer' },
  { email: 'agent@demo.capitec.local', role: 'Disputes agent' },
  { email: 'support@demo.capitec.local', role: 'Support' },
  { email: 'admin@demo.capitec.local', role: 'Administrator' },
]
const DEMO_PASSWORD = 'Demo@2026!'
const showDemoAccounts = import.meta.env.VITE_DEMO_MODE !== 'false'

function loginErrorMessage(error: unknown): string {
  if (!isApiError(error)) return 'Something went wrong. Please try again.'
  switch (error.code) {
    case 'INVALID_CREDENTIALS':
      return 'Incorrect email or password.'
    case 'ACCOUNT_LOCKED':
      return 'For your security, sign-in to this account is blocked. Please contact support to unlock it.'
    case 'RATE_LIMITED':
      return `Too many attempts from this device. Try again in ${error.retryAfterSeconds ?? 60} seconds.`
    default:
      return error.message
  }
}

export function LoginPage() {
  const { status, user, login, sessionExpired } = useAuth()
  const location = useLocation()
  const [submitError, setSubmitError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: { email: '', password: '' } })

  const from = (location.state as { from?: string } | null)?.from

  if (status === 'loading') return <Loading label="Checking your session" />
  // Signing in updates the session, which re-renders this page into the redirect below.
  if (user) return <Navigate to={from ?? homeFor(user.role)} replace />

  const onSubmit = async (values: FormValues) => {
    setSubmitError(null)
    try {
      await login(values.email, values.password)
    } catch (error) {
      setSubmitError(loginErrorMessage(error))
    }
  }

  return (
    <div className="login">
      <section className="login__panel" aria-labelledby="login-title">
        <h1 id="login-title">Sign in</h1>
        <p className="lede">Check your card transactions and follow your disputes.</p>

        {sessionExpired && !submitError && (
          <div className="notice notice--info" role="status">
            Your session ended. Sign in again to continue.
          </div>
        )}
        {submitError && (
          <div className="notice notice--error" role="alert">
            {submitError}
          </div>
        )}

        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <div className="field">
            <label htmlFor="email">Email address</label>
            <input
              id="email"
              type="email"
              autoComplete="username"
              aria-invalid={errors.email ? true : undefined}
              aria-describedby={errors.email ? 'email-error' : undefined}
              {...register('email')}
            />
            {errors.email && (
              <p id="email-error" className="field__error">
                {errors.email.message}
              </p>
            )}
          </div>
          <div className="field">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              aria-invalid={errors.password ? true : undefined}
              aria-describedby={errors.password ? 'password-error' : undefined}
              {...register('password')}
            />
            {errors.password && (
              <p id="password-error" className="field__error">
                {errors.password.message}
              </p>
            )}
          </div>
          <button type="submit" className="button button--primary button--wide" disabled={isSubmitting}>
            {isSubmitting ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </section>

      {showDemoAccounts && (
        <aside className="login__demo" aria-labelledby="demo-title">
          <h2 id="demo-title">Demo accounts</h2>
          <p>
            Every account uses the password <code>{DEMO_PASSWORD}</code>. Choose one to fill in the form.
          </p>
          <p>
            This is a demo environment with made-up customers. A production build hides this panel, and no demo accounts
            are created.
          </p>
          <ul>
            {demoAccounts.map((account) => (
              <li key={account.email}>
                <button
                  type="button"
                  className="demo-account"
                  onClick={() => {
                    setValue('email', account.email, { shouldValidate: true })
                    setValue('password', DEMO_PASSWORD, { shouldValidate: true })
                  }}
                >
                  <span>{account.role}</span>
                  <span className="demo-account__email">{account.email}</span>
                </button>
              </li>
            ))}
          </ul>
        </aside>
      )}
    </div>
  )
}
