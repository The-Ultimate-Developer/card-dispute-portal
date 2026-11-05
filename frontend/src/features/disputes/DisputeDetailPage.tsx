import { Link, useLocation, useParams } from 'react-router'

import { useDispute } from '../../api/hooks'
import { isApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { CaseTrack } from '../../components/CaseTrack'
import { ErrorNotice, Loading, Notice } from '../../components/Feedback'
import { StatusBadge } from '../../components/StatusBadge'
import { disputeReference, handledBy, reasonLabels, statusExplanation } from '../../lib/disputes'
import { formatDate, formatDateTime, formatMoney } from '../../lib/format'
import { NotFoundPage } from '../system/SystemPages'
import { StatusActions } from '../staff/StatusActions'

export function DisputeDetailPage() {
  const { id = '' } = useParams()
  const { user } = useAuth()
  const location = useLocation()
  const query = useDispute(id)

  const isStaff = user?.role === 'AGENT' || user?.role === 'ADMIN'
  const backTo = isStaff ? '/staff/disputes' : '/disputes'
  const justSubmitted = (location.state as { justSubmitted?: boolean } | null)?.justSubmitted

  if (query.isPending) return <Loading label="Loading dispute" />
  if (query.isError) {
    if (isApiError(query.error) && query.error.status === 404) return <NotFoundPage what="dispute" />
    return <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />
  }

  const dispute = query.data
  const handler = isStaff ? handledBy(dispute.timeline) : undefined

  return (
    <>
      <Link to={backTo} className="back-link">
        {isStaff ? 'Back to queue' : 'Back to my disputes'}
      </Link>

      {justSubmitted && (
        <Notice tone="success">
          Dispute submitted. We've emailed you a confirmation with reference {disputeReference(dispute.id)}.
        </Notice>
      )}

      <header className="page-header page-header--split">
        <div>
          <h1>{dispute.merchant}</h1>
          <p className="lede">
            <span className="numeric">{formatMoney(dispute.amountCents, dispute.currency)}</span> on{' '}
            {formatDate(dispute.transactionOccurredAt)}, card ending {dispute.cardLast4}
          </p>
        </div>
        <StatusBadge status={dispute.status} />
      </header>

      <div className="detail">
        <section className="detail__main" aria-labelledby="details-title">
          <h2 id="details-title">Details</h2>
          <dl className="summary">
            <div>
              <dt>Reference</dt>
              <dd>{disputeReference(dispute.id)}</dd>
            </div>
            {dispute.customer && (
              <div>
                <dt>Customer</dt>
                <dd>
                  {dispute.customer.fullName}
                  <br />
                  <span className="muted">{dispute.customer.email}</span>
                </dd>
              </div>
            )}
            {isStaff && (
              <div>
                <dt>Handled by</dt>
                <dd>
                  {handler ? (
                    <>
                      {handler.fullName}
                      <br />
                      <span className="muted">{handler.email}</span>
                    </>
                  ) : (
                    <span className="muted">Not picked up yet</span>
                  )}
                </dd>
              </div>
            )}
            <div>
              <dt>Reason</dt>
              <dd>{reasonLabels[dispute.reason]}</dd>
            </div>
            <div>
              <dt>{isStaff ? "Customer's description" : 'What you told us'}</dt>
              <dd className="prose">{dispute.description}</dd>
            </div>
            <div>
              <dt>Submitted</dt>
              <dd>{formatDateTime(dispute.createdAt)}</dd>
            </div>
          </dl>

          {isStaff && <StatusActions dispute={dispute} />}
        </section>

        <section className="detail__track" aria-labelledby="progress-title">
          <h2 id="progress-title">Progress</h2>
          {!isStaff && <p className="muted">{statusExplanation(dispute.status)}</p>}
          <CaseTrack status={dispute.status} timeline={dispute.timeline} viewer={isStaff ? 'staff' : 'customer'} />
        </section>
      </div>
    </>
  )
}
