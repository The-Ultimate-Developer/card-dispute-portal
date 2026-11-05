import { describe, expect, it } from 'vitest'

import { dispute } from '../test/fixtures'
import { caseTrack, disputeReference, handledBy, isFinal, nextStatuses, statusLabel } from './disputes'

describe('dispute state machine', () => {
  it('offers only the transitions the API allows', () => {
    expect(nextStatuses('SUBMITTED')).toEqual(['UNDER_REVIEW', 'REJECTED'])
    expect(nextStatuses('UNDER_REVIEW')).toEqual(['RESOLVED', 'REJECTED'])
    expect(nextStatuses('RESOLVED')).toEqual([])
    expect(nextStatuses('REJECTED')).toEqual([])
  })

  it('treats resolved and declined as final', () => {
    expect(isFinal('RESOLVED')).toBe(true)
    expect(isFinal('REJECTED')).toBe(true)
    expect(isFinal('UNDER_REVIEW')).toBe(false)
  })

  it('shows rejected disputes to customers as declined', () => {
    expect(statusLabel('REJECTED')).toBe('Declined')
  })

  it('builds a short reference from the id', () => {
    expect(disputeReference('d15e0a7b-0000-4000-8000-000000000201')).toBe('D15E0A7B')
  })
})

describe('caseTrack', () => {
  it('marks a new dispute as current and shows the steps still to come', () => {
    const { status, timeline } = dispute()
    const steps = caseTrack(status, timeline)
    expect(steps.map((s) => [s.title, s.state])).toEqual([
      ['Submitted', 'current'],
      ['Under review', 'upcoming'],
      ['Outcome', 'upcoming'],
    ])
    expect(steps[0]?.actor).toBe('You')
  })

  it('has no upcoming steps once the dispute is closed', () => {
    const steps = caseTrack('RESOLVED', [
      ...dispute().timeline,
      { id: 2, fromStatus: 'SUBMITTED', toStatus: 'UNDER_REVIEW', actorRole: 'AGENT', note: '', createdAt: '2026-10-22T08:00:00Z' },
      { id: 3, fromStatus: 'UNDER_REVIEW', toStatus: 'RESOLVED', actorRole: 'AGENT', note: 'Refunded', createdAt: '2026-10-23T08:00:00Z' },
    ])
    expect(steps.every((s) => s.state === 'done')).toBe(true)
    expect(steps.at(-1)?.actor).toBe('Disputes team')
  })
})

describe('who handled a dispute', () => {
  const reviewed = [
    ...dispute().timeline,
    {
      id: 2,
      fromStatus: 'SUBMITTED' as const,
      toStatus: 'UNDER_REVIEW' as const,
      actorRole: 'AGENT' as const,
      actor: { fullName: 'Lerato Dlamini', email: 'agent@demo.capitec.local' },
      note: '',
      createdAt: '2026-10-22T08:00:00Z',
    },
  ]

  it('names staff to staff, and only "Disputes team" to customers', () => {
    expect(caseTrack('UNDER_REVIEW', reviewed, 'staff').map((s) => s.actor)).toEqual(['Customer', 'Lerato Dlamini', undefined])
    expect(caseTrack('UNDER_REVIEW', reviewed, 'customer').map((s) => s.actor)).toEqual(['You', 'Disputes team', undefined])
  })

  it('is the last staff member who acted', () => {
    expect(handledBy(reviewed)?.fullName).toBe('Lerato Dlamini')
    expect(handledBy(dispute().timeline)).toBeUndefined()
  })
})
