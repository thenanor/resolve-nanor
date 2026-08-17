export type TicketStatus =
  | 'new'
  | 'open'
  | 'in_progress'
  | 'waiting_customer'
  | 'resolved'
  | 'closed'

export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'

export type TicketCategory = 'billing' | 'bug' | 'account_access' | 'feature_request' | 'how_to' | 'other'

export interface TicketComment {
  id: string
  author: string
  body: string
  internal: boolean
  at: string
}

export interface Ticket {
  id: string
  subject: string
  description: string
  customerEmail: string
  priority: TicketPriority
  category: TicketCategory | null
  status: TicketStatus
  comments: TicketComment[]
  createdAt: string
  updatedAt: string
  resolvedAt: string | null
  // A low-confidence triage suggestion awaiting human review — set together
  // or not at all. category/priority above are untouched until reviewed.
  pendingCategory: TicketCategory | null
  pendingPriority: TicketPriority | null
}

export interface AuditEntry {
  actor: string
  action: string
  ticketId: string
  details: Record<string, unknown>
  at: string
}

export interface Stats {
  total: number
  byStatus: Record<string, number>
  byPriority: Record<string, number>
  avgResolutionMinutes: number | null
}

export interface ApiError {
  message: string
}

export interface CannedResponse {
  id: string
  title: string
  body: string
  tags: string[]
  createdAt: string
  updatedAt: string
}
