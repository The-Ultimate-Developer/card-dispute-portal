import { Link } from 'react-router'

import { useOverview } from '../../api/hooks'
import type { DisputeStatus } from '../../api/types'
import { ErrorNotice, Loading } from '../../components/Feedback'
import { statusLabel } from '../../lib/disputes'
import { formatAge, formatMoney } from '../../lib/format'

const statusOrder: DisputeStatus[] = ['SUBMITTED', 'UNDER_REVIEW', 'RESOLVED', 'REJECTED']

export function OverviewPage() {
  const query = useOverview()

  if (query.isPending) return <Loading label="Loading overview" />
  if (query.isError) return <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />

  const o = query.data
  const total = statusOrder.reduce((sum, s) => sum + (o.disputesByStatus[s] ?? 0), 0)

  return (
    <>
      <header className="page-header">
        <h1>Overview</h1>
        <p className="lede">Refreshes every 30 seconds.</p>
      </header>

      <section aria-labelledby="disputes-title" className="overview-section">
        <h2 id="disputes-title">Disputes</h2>
        <dl className="figures">
          <div>
            <dt>Open</dt>
            <dd>{o.openDisputes}</dd>
          </div>
          <div>
            <dt>Value in dispute</dt>
            <dd>{formatMoney(o.openDisputedAmountCents)}</dd>
          </div>
          <div>
            <dt>Oldest open</dt>
            <dd>{o.oldestOpenDisputeAt ? formatAge(o.oldestOpenDisputeAt) : 'None'}</dd>
          </div>
          <div>
            <dt>New this week</dt>
            <dd>{o.disputesLast7Days}</dd>
          </div>
        </dl>

        {total > 0 && (
          <div className="distribution">
            <div className="distribution__bar" aria-hidden="true">
              {statusOrder.map((s) => {
                const count = o.disputesByStatus[s] ?? 0
                return count > 0 ? (
                  <span
                    key={s}
                    className={`distribution__segment distribution__segment--${s.toLowerCase()}`}
                    style={{ flexGrow: count }}
                  />
                ) : null
              })}
            </div>
            <ul className="distribution__legend">
              {statusOrder.map((s) => (
                <li key={s}>
                  <span className={`legend-dot legend-dot--${s.toLowerCase()}`} aria-hidden="true" />
                  <Link to={`/staff/disputes?status=${s}`}>{statusLabel(s)}</Link>
                  <span className="numeric">{o.disputesByStatus[s] ?? 0}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </section>

      <section aria-labelledby="security-title" className="overview-section">
        <h2 id="security-title">Accounts and sign-in</h2>
        <dl className="figures">
          <div>
            <dt>Customers</dt>
            <dd>{o.customers}</dd>
          </div>
          <div>
            <dt>
              <Link to="/staff/accounts">Locked accounts</Link>
            </dt>
            <dd>{o.lockedAccounts}</dd>
          </div>
          <div>
            <dt>
              <Link to="/admin/audit?event=LOGIN_FAILED">Failed sign-ins, 24 h</Link>
            </dt>
            <dd>{o.failedLoginsLast24h}</dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="emails-title" className="overview-section">
        <h2 id="emails-title">Customer emails</h2>
        <dl className="figures">
          <div>
            <dt>Waiting to send</dt>
            <dd>{o.notificationsPending}</dd>
          </div>
          <div className={o.notificationsFailed > 0 ? 'figures__alert' : undefined}>
            <dt>
              <Link to="/admin/notifications?status=DEAD">Failed after retries</Link>
            </dt>
            <dd>{o.notificationsFailed}</dd>
          </div>
        </dl>
      </section>
    </>
  )
}
