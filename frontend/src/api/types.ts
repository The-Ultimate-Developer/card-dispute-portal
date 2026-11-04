// Friendly names over the types generated from backend/api/openapi.yaml.
// Run `npm run gen:api` after the contract changes; CI fails if they drift.
import type { components } from './schema'

type Schemas = components['schemas']

export type Problem = Schemas['Problem']
export type Role = Schemas['Role']
export type User = Schemas['User']
export type Session = Schemas['Session']
export type Transaction = Schemas['Transaction']
export type TransactionPage = Schemas['TransactionPage']
export type Dispute = Schemas['Dispute']
export type DisputeDetail = Schemas['DisputeDetail']
export type DisputePage = Schemas['DisputePage']
export type DisputeStatus = Schemas['DisputeStatus']
export type DisputeReason = Schemas['DisputeReason']
export type TimelineEvent = Schemas['TimelineEvent']
export type CreateDisputeRequest = Schemas['CreateDisputeRequest']
export type ChangeStatusRequest = Schemas['ChangeStatusRequest']
export type LockedAccount = Schemas['LockedAccount']
export type Overview = Schemas['Overview']
export type AuditEvent = Schemas['AuditEvent']
export type AuditEventType = Schemas['AuditEventType']
export type AuditPage = Schemas['AuditPage']
export type Notification = Schemas['Notification']
export type NotificationPage = Schemas['NotificationPage']
