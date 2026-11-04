import { Link, useSearchParams } from 'react-router'

import { useDisputes } from '../../api/hooks'
import type { DisputeStatus } from '../../api/types'
import { EmptyState, ErrorNotice, LoadMore, Loading } from '../../components/Feedback'
import { StatusBadge } from '../../components/StatusBadge'
import { disputeReference, reasonLabels, statusLabel } from '../../lib/disputes'
import { formatAge, formatMoney } from '../../lib/format'

const filters: { value: DisputeStatus | 'ALL'; label: string }[] = [
  { value: 'SUBMITTED', label: statusLabel('SUBMITTED') },
  { value: 'UNDER_REVIEW', label: statusLabel('UNDER_REVIEW') },
  { value: 'RESOLVED', label: statusLabel('RESOLVED') },
  { value: 'REJECTED', label: statusLabel('REJECTED') },
  { value: 'ALL', label: 'All' },
]

function parseFilter(value: string | null): DisputeStatus | 'ALL' {
  return filters.some((f) => f.value === value) ? (value as DisputeStatus | 'ALL') : 'SUBMITTED'
}

export function QueuePage() {
  // The filter lives in the URL, so a filtered queue can be bookmarked and Back works.
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('status'))
  const query = useDisputes(filter === 'ALL' ? undefined : filter)
  const disputes = query.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <>
      <header className="page-header">
        <h1>Dispute queue</h1>
        <p className="lede">New disputes wait in Submitted. Start a review, contact the merchant, then record the outcome.</p>
      </header>

      <div className="filters" role="tablist" aria-label="Filter by status">
        {filters.map((f) => (
          <button
            key={f.value}
            type="button"
            role="tab"
            aria-selected={filter === f.value}
            className="filters__tab"
            onClick={() => setParams(f.value === 'SUBMITTED' ? {} : { status: f.value })}
          >
            {f.label}
          </button>
        ))}
      </div>

      {query.isPending && <Loading label="Loading disputes" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && disputes.length === 0 && (
        <EmptyState title={filter === 'SUBMITTED' ? 'No new disputes' : 'Nothing here'}>
          <p>{filter === 'SUBMITTED' ? 'New disputes from customers will appear here.' : 'No disputes have this status.'}</p>
        </EmptyState>
      )}

      {disputes.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Reference</th>
                <th scope="col">Customer</th>
                <th scope="col">Merchant</th>
                <th scope="col">Reason</th>
                <th scope="col" className="table__numeric">
                  Amount
                </th>
                <th scope="col">Waiting</th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {disputes.map((dispute) => (
                <tr key={dispute.id}>
                  <td>
                    <Link to={`/staff/disputes/${dispute.id}`}>{disputeReference(dispute.id)}</Link>
                  </td>
                  <td>{dispute.customer?.fullName ?? 'Unknown'}</td>
                  <td>{dispute.merchant}</td>
                  <td>{reasonLabels[dispute.reason]}</td>
                  <td className="table__numeric">{formatMoney(dispute.amountCents, dispute.currency)}</td>
                  <td>{formatAge(dispute.createdAt)}</td>
                  <td>
                    <StatusBadge status={dispute.status} />
                  </td>
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
