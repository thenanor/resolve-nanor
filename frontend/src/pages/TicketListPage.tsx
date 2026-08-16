import { useEffect, useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { CategoryBadge, NeedsReviewBadge, PriorityBadge, StatusBadge } from '../components/Badge'
import type { Ticket, TicketPriority, TicketStatus } from '../types'

const STATUSES: TicketStatus[] = ['new', 'open', 'in_progress', 'waiting_customer', 'resolved', 'closed']
const PRIORITIES: TicketPriority[] = ['low', 'normal', 'high', 'urgent']

export function TicketListPage() {
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [status, setStatus] = useState('')
  const [priority, setPriority] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    api
      .listTickets({ status, priority })
      .then(setTickets)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [status, priority])

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
        <h1 style={{ margin: 0, fontSize: 24 }}>Tickets</h1>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">All statuses</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <select value={priority} onChange={(e) => setPriority(e.target.value)}>
          <option value="">All priorities</option>
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
        <Link to="/tickets/new" style={{ marginLeft: 'auto' }}>
          + New Ticket
        </Link>
      </div>

      {error && <p style={{ color: '#dc2626' }}>{error}</p>}
      {loading ? (
        <p>Loading…</p>
      ) : tickets.length === 0 ? (
        <p style={{ color: '#6b7280' }}>No tickets found.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ textAlign: 'left', borderBottom: '1px solid #e5e7eb' }}>
              <th style={th}>Subject</th>
              <th style={th}>Customer</th>
              <th style={th}>Status</th>
              <th style={th}>Priority</th>
              <th style={th}>Category</th>
              <th style={th}>Created</th>
            </tr>
          </thead>
          <tbody>
            {tickets.map((t) => (
              <tr key={t.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                <td style={td}>
                  <Link to={`/tickets/${t.id}`}>{t.subject}</Link>
                </td>
                <td style={td}>{t.customerEmail}</td>
                <td style={td}>
                  <StatusBadge status={t.status} />
                </td>
                <td style={td}>
                  <PriorityBadge priority={t.priority} />
                </td>
                <td style={td}>
                  {t.category && <CategoryBadge category={t.category} />}
                  {t.pendingCategory && <NeedsReviewBadge />}
                </td>
                <td style={td}>{new Date(t.createdAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

const th: CSSProperties = { padding: '8px 4px', fontSize: 15, color: '#6b7280' }
const td: CSSProperties = { padding: '8px 4px', fontSize: 16 }
