import type { ApiError, AuditEntry, Stats, Ticket, TicketComment } from '../types'

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

export const api = {
  listTickets: (filter: { status?: string; priority?: string } = {}) => {
    const params = new URLSearchParams()
    if (filter.status) params.set('status', filter.status)
    if (filter.priority) params.set('priority', filter.priority)
    const qs = params.toString()
    return request<Ticket[]>(`/tickets${qs ? `?${qs}` : ''}`)
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

  listAudit: (ticketId?: string) =>
    request<AuditEntry[]>(`/audit${ticketId ? `?ticketId=${ticketId}` : ''}`),

  getStats: () => request<Stats>('/stats'),
}

export { ApiRequestError }
