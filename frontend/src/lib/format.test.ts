import { describe, expect, it } from 'vitest'

import { formatAge, formatMoney } from './format'

describe('formatMoney', () => {
  it.each([
    [0, 'R0.00'],
    [5, 'R0.05'],
    [129_999, 'R1\u2009299.99'],
    [123_456_789, 'R1\u2009234\u2009567.89'],
    [-2_500, '-R25.00'],
  ])('formats %i cents as %s', (cents, expected) => {
    expect(formatMoney(cents)).toBe(expected)
  })

  it('prefixes other currencies with their code', () => {
    expect(formatMoney(1_000, 'USD')).toBe('USD 10.00')
  })
})

describe('formatAge', () => {
  const now = new Date('2026-11-01T12:00:00Z')

  it('uses minutes, then hours, then days', () => {
    expect(formatAge('2026-11-01T11:45:00Z', now)).toBe('15 min')
    expect(formatAge('2026-11-01T07:00:00Z', now)).toBe('5 h')
    expect(formatAge('2026-10-28T12:00:00Z', now)).toBe('4 days')
  })

  it('never shows a negative age when clocks disagree', () => {
    expect(formatAge('2026-11-01T12:05:00Z', now)).toBe('0 min')
  })
})
