import { useState } from 'react'
import { Link } from 'react-router'

import { useTransactions } from '../../api/hooks'
import type { Transaction } from '../../api/types'
import { Dialog } from '../../components/Dialog'
import { EmptyState, ErrorNotice, LoadMore, Loading } from '../../components/Feedback'
import { StatusBadge } from '../../components/StatusBadge'
import { formatDate, formatMoney } from '../../lib/format'
import { DisputeForm } from './DisputeForm'

export function TransactionsPage() {
  const query = useTransactions()
  const [disputing, setDisputing] = useState<Transaction | null>(null)

  const transactions = query.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <>
      <header className="page-header">
        <h1>Transactions</h1>
        <p className="lede">Card payments from the last few months. Dispute any payment you don't recognise or that went wrong.</p>
      </header>

      {query.isPending && <Loading label="Loading your transactions" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && transactions.length === 0 && (
        <EmptyState title="No card transactions yet">
          <p>Payments made with your cards will appear here.</p>
        </EmptyState>
      )}

      {transactions.length > 0 && (
        <ul className="statement" aria-label="Transactions">
          {transactions.map((transaction) => (
            <li key={transaction.id} className="statement__row">
              <time className="statement__date" dateTime={transaction.occurredAt}>
                {formatDate(transaction.occurredAt)}
              </time>
              <div className="statement__what">
                <span className="statement__merchant">{transaction.merchant}</span>
                <span className="statement__detail">
                  <span>{transaction.category}</span>
                  <span>
                    {transaction.cardBrand} ending {transaction.cardLast4}
                  </span>
                </span>
              </div>
              <span className="statement__amount">{formatMoney(transaction.amountCents, transaction.currency)}</span>
              <div className="statement__action">
                <TransactionAction transaction={transaction} onDispute={() => setDisputing(transaction)} />
              </div>
            </li>
          ))}
        </ul>
      )}

      <LoadMore
        hasNextPage={query.hasNextPage}
        isFetchingNextPage={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
      />

      <Dialog title="Dispute this payment" open={disputing !== null} onClose={() => setDisputing(null)}>
        {disputing && <DisputeForm transaction={disputing} onCancel={() => setDisputing(null)} />}
      </Dialog>
    </>
  )
}

function TransactionAction({ transaction, onDispute }: { transaction: Transaction; onDispute: () => void }) {
  // A closed dispute stays visible, and a payment can be disputed again once the last one is closed.
  const existing = transaction.dispute && (
    <Link to={`/disputes/${transaction.dispute.id}`} className="statement__link">
      <StatusBadge status={transaction.dispute.status} />
      <span className="visually-hidden">View dispute for {transaction.merchant}</span>
    </Link>
  )
  if (transaction.disputable) {
    return (
      <>
        {existing}
        <button type="button" className="button button--small" onClick={onDispute}>
          Dispute<span className="visually-hidden"> payment to {transaction.merchant}</span>
        </button>
      </>
    )
  }
  if (existing) return existing
  if (transaction.status === 'PENDING') {
    return <span className="statement__hint">Pending</span>
  }
  return null
}
