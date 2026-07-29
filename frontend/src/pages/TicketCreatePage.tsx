import { useState, type CSSProperties, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { TicketPriority } from '../types'

export function TicketCreatePage() {
  const navigate = useNavigate()
  const [subject, setSubject] = useState('')
  const [description, setDescription] = useState('')
  const [customerEmail, setCustomerEmail] = useState('')
  const [priority, setPriority] = useState<TicketPriority>('normal')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const ticket = await api.createTicket({ subject, description, customerEmail, priority })
      navigate(`/tickets/${ticket.id}`)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 480 }}>
      <h1 style={{ fontSize: 24 }}>New Ticket</h1>
      <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <label>
          Subject
          <input value={subject} onChange={(e) => setSubject(e.target.value)} style={input} />
        </label>
        <label>
          Description
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={4}
            style={input}
          />
        </label>
        <label>
          Customer email
          <input
            type="email"
            value={customerEmail}
            onChange={(e) => setCustomerEmail(e.target.value)}
            style={input}
          />
        </label>
        <label>
          Priority
          <select
            value={priority}
            onChange={(e) => setPriority(e.target.value as TicketPriority)}
            style={input}
          >
            <option value="low">low</option>
            <option value="normal">normal</option>
            <option value="high">high</option>
            <option value="urgent">urgent</option>
          </select>
        </label>

        {error && <p style={{ color: '#dc2626' }}>{error}</p>}

        <button type="submit" disabled={submitting} style={{ padding: '8px 16px' }}>
          {submitting ? 'Creating…' : 'Create Ticket'}
        </button>
      </form>
    </div>
  )
}

const input: CSSProperties = {
  display: 'block',
  width: '100%',
  marginTop: 4,
  padding: '6px 8px',
  border: '1px solid #d1d5db',
  borderRadius: 6,
  boxSizing: 'border-box',
}
