import type { DisputeStatus } from '../api/types'
import { statusLabel } from '../lib/disputes'

export function StatusBadge({ status }: { status: DisputeStatus }) {
  return <span className={`badge badge--${status.toLowerCase()}`}>{statusLabel(status)}</span>
}
