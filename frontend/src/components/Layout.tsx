import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router'

import { navigation, roleLabels } from '../auth/access'
import { useAuth } from '../auth/AuthProvider'

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [signingOut, setSigningOut] = useState(false)

  const signOut = async () => {
    setSigningOut(true)
    try {
      await logout()
    } finally {
      await navigate('/login', { replace: true })
    }
  }

  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <header className="topbar">
        <div className="topbar__inner">
          <span className="brand">
            <svg viewBox="0 0 32 32" aria-hidden="true">
              <rect x="3" y="7" width="26" height="18" rx="3" />
              <rect x="3" y="11" width="26" height="3" className="brand__stripe" />
            </svg>
            Card disputes
          </span>
          {user && (
            <nav className="nav" aria-label="Main">
              {navigation[user.role].map((item) => (
                <NavLink key={item.to} to={item.to} end={item.to === '/admin'} className="nav__link">
                  {item.label}
                </NavLink>
              ))}
            </nav>
          )}
          {user && (
            <div className="account">
              <span className="account__name">
                {user.fullName}
                <span className="account__role">{roleLabels[user.role]}</span>
              </span>
              <button type="button" className="button button--quiet" onClick={signOut} disabled={signingOut}>
                Sign out
              </button>
            </div>
          )}
        </div>
      </header>
      <main id="main" className="content">
        <Outlet />
      </main>
    </div>
  )
}
