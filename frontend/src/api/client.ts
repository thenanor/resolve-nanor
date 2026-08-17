import type { ApiError, AuditEntry, CannedResponse, Stats, Ticket, TicketComment } from '../types'

const ACTOR_KEY = 'resolve.actor'

export function getActor(): string {
  return localStorage.getItem(ACTOR_KEY) || 'api'
}

export function setActor(actor: string): void {
  localStorage.setItem(ACTOR_KEY, actor || 'api')
}

class ApiRequestError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Actor': getActor(),
      ...init?.headers,
    },
  })
  if (!res.ok) {
    let message = `request failed with status ${res.status}`
    try {
      const body = (await res.json()) as ApiError
      if (body?.message) message = body.message
    } catch {
      // ignore body parse failures, fall back to the generic message
    }
    throw new ApiRequestError(message, res.status)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export interface CreateTicketInput {
  subject: string
  description: string
  customerEmail: string
  priority: string
}

export interface AddCommentInput {
  author: string
  body: string
  internal: boolean
}

interface TicketPage {
  tickets: Ticket[]
  limit: number
  offset: number
  hasMore: boolean
}

export interface SaveCannedResponseInput {
  title: string
  body: string
  tags: string[]
}

export const api = {
  // GET /tickets now returns a page envelope (tickets + limit/offset/hasMore)
  // rather than a bare array, so offset pagination can later grow a cursor
  // field without another breaking response-shape change. This call still
  // returns Ticket[] since the list page doesn't have pagination controls yet.
  listTickets: (filter: { status?: string; priority?: string; limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams()
    if (filter.status) params.set('status', filter.status)
    if (filter.priority) params.set('priority', filter.priority)
    if (filter.limit != null) params.set('limit', String(filter.limit))
    if (filter.offset != null) params.set('offset', String(filter.offset))
    const qs = params.toString()
    return request<TicketPage>(`/tickets${qs ? `?${qs}` : ''}`).then((page) => page.tickets)
  },

  getTicket: (id: string) => request<Ticket>(`/tickets/${id}`),

  createTicket: (input: CreateTicketInput) =>
    request<Ticket>('/tickets', { method: 'POST', body: JSON.stringify(input) }),

  changeStatus: (id: string, to: string) =>
    request<Ticket>(`/tickets/${id}/status`, {
      method: 'POST',
      body: JSON.stringify({ to }),
    }),

  addComment: (id: string, input: AddCommentInput) =>
    request<TicketComment>(`/tickets/${id}/comments`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  reviewTriage: (id: string, decision: 'accept' | 'reject') =>
    request<Ticket>(`/tickets/${id}/triage/review`, {
      method: 'POST',
      body: JSON.stringify({ decision }),
    }),

  listAudit: (ticketId?: string) =>
    request<AuditEntry[]>(`/audit${ticketId ? `?ticketId=${ticketId}` : ''}`),

  getStats: () => request<Stats>('/stats'),

  listCannedResponses: (tag?: string) =>
    request<CannedResponse[]>(`/canned-responses${tag ? `?tag=${encodeURIComponent(tag)}` : ''}`),

  createCannedResponse: (input: SaveCannedResponseInput) =>
    request<CannedResponse>('/canned-responses', { method: 'POST', body: JSON.stringify(input) }),

  updateCannedResponse: (id: string, input: SaveCannedResponseInput) =>
    request<CannedResponse>(`/canned-responses/${id}`, { method: 'PUT', body: JSON.stringify(input) }),

  deleteCannedResponse: (id: string) =>
    request<void>(`/canned-responses/${id}`, { method: 'DELETE' }),
}

export { ApiRequestError }
