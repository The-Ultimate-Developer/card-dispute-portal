import { Link } from 'react-router'

import { homeFor } from '../../auth/access'
import { useAuth } from '../../auth/AuthProvider'

function HomeLink() {
  const { user } = useAuth()
  return <Link to={user ? homeFor(user.role) : '/login'}>Go to your home page</Link>
}

export function ForbiddenPage() {
  return (
    <div className="system-page">
      <h1>You don't have access to this page</h1>
      <p>It belongs to a different role. If you need access, ask an administrator.</p>
      <HomeLink />
    </div>
  )
}

export function NotFoundPage({ what = 'page' }: { what?: string }) {
  return (
    <div className="system-page">
      <h1>We couldn't find that {what}</h1>
      <p>The link may be wrong, or it may belong to a different account.</p>
      <HomeLink />
    </div>
  )
}
