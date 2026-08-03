import type { TicketStatus } from '../types'

// Mirrors ALLOWED_TRANSITIONS on the backend. Used only to disable buttons
// for moves that would be rejected — the backend remains the source of
// truth for validation.
export const ALLOWED_TRANSITIONS: Record<TicketStatus, TicketStatus[]> = {
  new: ['open'],
  open: ['in_progress'],
  in_progress: ['waiting_customer', 'resolved'],
  waiting_customer: ['in_progress'],
  resolved: ['closed', 'in_progress'],
  closed: [],
}
