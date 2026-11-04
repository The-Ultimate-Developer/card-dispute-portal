import type { DisputeStatus, TimelineEvent } from '../api/types'
import { caseTrack } from '../lib/disputes'
import { formatDateTime } from '../lib/format'

interface CaseTrackProps {
  status: DisputeStatus
  timeline: TimelineEvent[]
  viewer: 'customer' | 'staff'
}

export function CaseTrack({ status, timeline, viewer }: CaseTrackProps) {
  const steps = caseTrack(status, timeline)
  return (
    <ol className="track" aria-label="Dispute progress">
      {steps.map((step) => (
        <li
          key={step.key}
          className={`track__step track__step--${step.state}${step.status ? ` track__step--${step.status.toLowerCase()}` : ''}`}
          aria-current={step.state === 'current' ? 'step' : undefined}
        >
          <span className="track__node" aria-hidden="true" />
          <div className="track__body">
            <p className="track__title">
              {step.title}
              {step.state === 'upcoming' && <span className="visually-hidden"> (not yet reached)</span>}
            </p>
            {step.at && (
              <p className="track__meta">
                <time dateTime={step.at}>{formatDateTime(step.at)}</time>
                {step.actor && <> by {viewer === 'staff' && step.actor === 'You' ? 'Customer' : step.actor}</>}
              </p>
            )}
            {step.note && <p className="track__note">{step.note}</p>}
          </div>
        </li>
      ))}
    </ol>
  )
}
