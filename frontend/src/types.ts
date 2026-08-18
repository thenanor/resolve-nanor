export type TicketStatus =
  | 'new'
  | 'open'
  | 'in_progress'
  | 'waiting_customer'
  | 'resolved'
  | 'closed'

export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'

export type TicketCategory = 'billing' | 'bug' | 'account_access' | 'feature_request' | 'how_to' | 'other'

export type ReplyGuardVerdict = 'send' | 'revise' | 'escalate'
export type ReplyGuardPolicy = 'disclosure' | 'commitment' | 'answer' | 'tone'
export type ReplyGuardSeverity = 'low' | 'medium' | 'high'

export interface ReplyGuardFinding {
  policy: ReplyGuardPolicy
  severity: ReplyGuardSeverity
  issue: string
  quote: string
}

export interface ReplyGuardResult {
  verdict: ReplyGuardVerdict
  findings: ReplyGuardFinding[]
  confidence: 'low' | 'medium' | 'high'
  reasoning: string
  injectionSuspected: boolean
  requireHuman: boolean
}

// The 409 error body returned by POST /tickets/{id}/comments when
// reply-guard rejects a reply (revise with no overrideReason, or escalate).
export interface GuardRejection extends ReplyGuardResult {
  message: string
}

export interface TicketComment {
  id: string
  author: string
  body: string
  internal: boolean
  at: string
  // Present only on the Comment returned immediately by addComment, when
  // reply-guard actually ran for it — always null on a comment read back
  // later (e.g. embedded in Ticket.comments), since it's never persisted.
  guardResult: ReplyGuardResult | null
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
