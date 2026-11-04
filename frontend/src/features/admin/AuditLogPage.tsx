import { useSearchParams } from 'react-router'

import { useAuditEvents } from '../../api/hooks'
import type { AuditEventType } from '../../api/types'
import { EmptyState, ErrorNotice, LoadMore, Loading } from '../../components/Feedback'
import { formatDateTime } from '../../lib/format'

const eventLabels: Record<AuditEventType, string> = {
  LOGIN_SUCCEEDED: 'Signed in',
  LOGIN_FAILED: 'Wrong password',
  LOGIN_BLOCKED: 'Blocked (account locked)',
  ACCOUNT_LOCKED: 'Account locked',
  ACCOUNT_UNLOCKED: 'Account unlocked',
  LOGOUT: 'Signed out',
}
const eventTypes = Object.keys(eventLabels) as AuditEventType[]

export function AuditLogPage() {
  const [params, setParams] = useSearchParams()
  const raw = params.get('event')
  const event = eventTypes.find((e) => e === raw)
  const query = useAuditEvents(event)
  const events = query.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <>
      <header className="page-header">
        <h1>Sign-in activity</h1>
        <p className="lede">Every sign-in, lockout and unlock. This record can't be edited or deleted.</p>
      </header>

      <div className="toolbar">
        <label htmlFor="event-filter">Show</label>
        <select
          id="event-filter"
          value={event ?? ''}
          onChange={(e) => setParams(e.target.value ? { event: e.target.value } : {})}
        >
          <option value="">All activity</option>
          {eventTypes.map((e) => (
            <option key={e} value={e}>
              {eventLabels[e]}
            </option>
          ))}
        </select>
      </div>

      {query.isPending && <Loading label="Loading activity" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && events.length === 0 && <EmptyState title="No activity matches this filter" />}

      {events.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">When</th>
                <th scope="col">What</th>
                <th scope="col">Account</th>
                <th scope="col">By staff member</th>
                <th scope="col">IP address</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id}>
                  <td>{formatDateTime(e.createdAt)}</td>
                  <td>{eventLabels[e.event]}</td>
                  <td>{e.email ?? 'Unknown'}</td>
                  <td>{e.actorEmail ?? <span className="muted">Customer</span>}</td>
                  <td className="numeric">{e.ip ?? <span className="muted">Not recorded</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <LoadMore
        hasNextPage={query.hasNextPage}
        isFetchingNextPage={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
      />
    </>
  )
}
