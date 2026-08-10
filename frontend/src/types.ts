export type TicketStatus =
  | 'new'
  | 'open'
  | 'in_progress'
  | 'waiting_customer'
  | 'resolved'
  | 'closed'

export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'

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
  status: TicketStatus
  comments: TicketComment[]
  createdAt: string
  updatedAt: string
  resolvedAt: string | null
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
