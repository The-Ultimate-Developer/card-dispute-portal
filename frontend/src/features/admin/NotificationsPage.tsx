import { useSearchParams } from 'react-router'

import { useNotifications } from '../../api/hooks'
import type { Notification } from '../../api/types'
import { EmptyState, ErrorNotice, LoadMore, Loading } from '../../components/Feedback'
import { formatDateTime } from '../../lib/format'

const topicLabels: Record<Notification['topic'], string> = {
  'dispute.created': 'Dispute received',
  'dispute.status_changed': 'Dispute update',
  'account.locked': 'Account locked warning',
}

const statusLabels: Record<Notification['status'], string> = {
  PENDING: 'Waiting',
  SENT: 'Sent',
  DEAD: 'Failed',
}

export function NotificationsPage() {
  const [params, setParams] = useSearchParams()
  const raw = params.get('status')
  const status = raw === 'PENDING' || raw === 'SENT' || raw === 'DEAD' ? raw : undefined
  const query = useNotifications(status)
  const items = query.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <>
      <header className="page-header">
        <h1>Emails</h1>
        <p className="lede">
          Delivery status of customer emails. Failed emails were retried with increasing delays before giving up. Message
          content isn't shown here.
        </p>
      </header>

      <div className="toolbar">
        <label htmlFor="status-filter">Show</label>
        <select id="status-filter" value={status ?? ''} onChange={(e) => setParams(e.target.value ? { status: e.target.value } : {})}>
          <option value="">All emails</option>
          <option value="PENDING">Waiting</option>
          <option value="SENT">Sent</option>
          <option value="DEAD">Failed</option>
        </select>
      </div>

      {query.isPending && <Loading label="Loading emails" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && items.length === 0 && <EmptyState title="No emails match this filter" />}

      {items.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Created</th>
                <th scope="col">Email</th>
                <th scope="col">Status</th>
                <th scope="col" className="table__numeric">
                  Attempts
                </th>
                <th scope="col">Sent or next try</th>
                <th scope="col">Last error</th>
              </tr>
            </thead>
            <tbody>
              {items.map((n) => (
                <tr key={n.id}>
                  <td>{formatDateTime(n.createdAt)}</td>
                  <td>{topicLabels[n.topic]}</td>
                  <td>
                    <span className={`pill pill--${n.status.toLowerCase()}`}>{statusLabels[n.status]}</span>
                  </td>
                  <td className="table__numeric">{n.attempts}</td>
                  <td>
                    {n.sentAt ? formatDateTime(n.sentAt) : n.nextAttemptAt ? formatDateTime(n.nextAttemptAt) : <span className="muted">None</span>}
                  </td>
                  <td className="table__wrap">{n.lastError ?? <span className="muted">None</span>}</td>
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
