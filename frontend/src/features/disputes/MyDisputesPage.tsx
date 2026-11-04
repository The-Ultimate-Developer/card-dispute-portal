import { Link } from 'react-router'

import { useDisputes } from '../../api/hooks'
import { EmptyState, ErrorNotice, LoadMore, Loading } from '../../components/Feedback'
import { StatusBadge } from '../../components/StatusBadge'
import { disputeReference, reasonLabels } from '../../lib/disputes'
import { formatDate, formatMoney } from '../../lib/format'

export function MyDisputesPage() {
  const query = useDisputes()
  const disputes = query.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <>
      <header className="page-header">
        <h1>My disputes</h1>
        <p className="lede">Every payment you've disputed, newest first. Open one to see its progress.</p>
      </header>

      {query.isPending && <Loading label="Loading your disputes" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && disputes.length === 0 && (
        <EmptyState title="You haven't disputed any payments">
          <p>
            To start, go to <Link to="/transactions">Transactions</Link> and choose Dispute next to the payment.
          </p>
        </EmptyState>
      )}

      {disputes.length > 0 && (
        <ul className="list" aria-label="Disputes">
          {disputes.map((dispute) => (
            <li key={dispute.id}>
              <Link to={`/disputes/${dispute.id}`} className="list__row">
                <div className="list__main">
                  <span className="list__title">{dispute.merchant}</span>
                  <span className="list__meta">
                    <span>{reasonLabels[dispute.reason]}</span>
                    <span>Submitted {formatDate(dispute.createdAt)}</span>
                    <span>Ref {disputeReference(dispute.id)}</span>
                  </span>
                </div>
                <span className="list__amount">{formatMoney(dispute.amountCents, dispute.currency)}</span>
                <StatusBadge status={dispute.status} />
              </Link>
            </li>
          ))}
        </ul>
      )}

      <LoadMore
        hasNextPage={query.hasNextPage}
        isFetchingNextPage={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
      />
    </>
  )
}
