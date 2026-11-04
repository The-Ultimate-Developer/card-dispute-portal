import type { DisputeReason, DisputeStatus, TimelineEvent } from '../api/types'

export function assertNever(value: never): never {
  throw new Error(`Unhandled value: ${String(value)}`)
}

// Exhaustive switches: adding a status to the OpenAPI enum makes these fail to
// compile until every place that shows a status has been updated.
export function statusLabel(status: DisputeStatus): string {
  switch (status) {
    case 'SUBMITTED':
      return 'Submitted'
    case 'UNDER_REVIEW':
      return 'Under review'
    case 'RESOLVED':
      return 'Resolved'
    case 'REJECTED':
      return 'Declined'
    default:
      return assertNever(status)
  }
}

export function statusExplanation(status: DisputeStatus): string {
  switch (status) {
    case 'SUBMITTED':
      return "We've received your dispute and an agent will pick it up shortly."
    case 'UNDER_REVIEW':
      return 'An agent is investigating, usually by contacting the merchant.'
    case 'RESOLVED':
      return 'This dispute is closed in your favour.'
    case 'REJECTED':
      return 'This dispute is closed. The reason is in the latest update below.'
    default:
      return assertNever(status)
  }
}

export const reasonLabels: Record<DisputeReason, string> = {
  UNRECOGNISED: "I don't recognise this payment",
  DUPLICATE: 'I was charged more than once',
  INCORRECT_AMOUNT: 'The amount is wrong',
  NOT_RECEIVED: "I didn't receive what I paid for",
  CANCELLED_SUBSCRIPTION: 'I cancelled this subscription',
  OTHER: 'Something else',
}

// Mirrors the backend state machine in internal/disputes/model.go. The server
// still enforces it; this only decides which buttons to offer.
const transitions: Record<DisputeStatus, DisputeStatus[]> = {
  SUBMITTED: ['UNDER_REVIEW', 'REJECTED'],
  UNDER_REVIEW: ['RESOLVED', 'REJECTED'],
  RESOLVED: [],
  REJECTED: [],
}

export function nextStatuses(status: DisputeStatus): DisputeStatus[] {
  return transitions[status]
}

export function isFinal(status: DisputeStatus): boolean {
  return transitions[status].length === 0
}

export function actionLabel(to: DisputeStatus): string {
  switch (to) {
    case 'UNDER_REVIEW':
      return 'Start review'
    case 'RESOLVED':
      return 'Resolve in favour of customer'
    case 'REJECTED':
      return 'Decline dispute'
    case 'SUBMITTED':
      return 'Reopen'
    default:
      return assertNever(to)
  }
}

export function disputeReference(id: string): string {
  return id.slice(0, 8).toUpperCase()
}

export interface TrackStep {
  key: string
  title: string
  state: 'done' | 'current' | 'upcoming'
  status?: DisputeStatus
  at?: string
  note?: string
  actor?: string
}

// The case track: every recorded event, then the steps still to come, so a
// customer always sees what happens next.
export function caseTrack(status: DisputeStatus, timeline: TimelineEvent[]): TrackStep[] {
  const steps: TrackStep[] = timeline.map((event, index) => ({
    key: `event-${event.id}`,
    title: statusLabel(event.toStatus),
    state: index === timeline.length - 1 && !isFinal(status) ? 'current' : 'done',
    status: event.toStatus,
    at: event.createdAt,
    note: event.note,
    actor: event.actorRole === 'CUSTOMER' ? 'You' : 'Disputes team',
  }))

  if (status === 'SUBMITTED') {
    steps.push({ key: 'next-review', title: 'Under review', state: 'upcoming' })
  }
  if (!isFinal(status)) {
    steps.push({ key: 'next-outcome', title: 'Outcome', state: 'upcoming' })
  }
  return steps
}
