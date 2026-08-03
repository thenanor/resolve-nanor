import { describe, expect, it } from 'vitest'
import { ALLOWED_TRANSITIONS } from './transitions'

describe('ALLOWED_TRANSITIONS', () => {
  it('walks the happy path forward', () => {
    expect(ALLOWED_TRANSITIONS.new).toEqual(['open'])
    expect(ALLOWED_TRANSITIONS.open).toEqual(['in_progress'])
    expect(ALLOWED_TRANSITIONS.in_progress).toEqual(['waiting_customer', 'resolved'])
    expect(ALLOWED_TRANSITIONS.resolved).toEqual(['closed', 'open'])
  })

  it('allows looping between in_progress and waiting_customer', () => {
    expect(ALLOWED_TRANSITIONS.waiting_customer).toEqual(['in_progress'])
  })

  it('allows reopening a resolved ticket', () => {
    expect(ALLOWED_TRANSITIONS.resolved).toContain('open')
  })

  it('treats closed as a terminal state', () => {
    expect(ALLOWED_TRANSITIONS.closed).toEqual([])
  })
})
