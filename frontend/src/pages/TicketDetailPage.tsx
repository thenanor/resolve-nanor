import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api/client'
import { PriorityBadge, StatusBadge } from '../components/Badge'
import { ALLOWED_TRANSITIONS } from '../lib/transitions'
import type { Ticket } from '../types'

export function TicketDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [ticket, setTicket] = useState<Ticket | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const [commentAuthor, setCommentAuthor] = useState('')
  const [commentBody, setCommentBody] = useState('')
  const [commentInternal, setCommentInternal] = useState(false)
  const [submittingComment, setSubmittingComment] = useState(false)
  const [statusBusy, setStatusBusy] = useState(false)

  const load = useCallback(() => {
    if (!id) return
    setLoading(true)
    setError(null)
    api
      .getTicket(id)
      .then(setTicket)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  useEffect(() => {
    load()
  }, [load])

  async function onChangeStatus(to: string) {
    if (!id) return
    setStatusBusy(true)
    setError(null)
    try {
      const updated = await api.changeStatus(id, to)
      setTicket(updated)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setStatusBusy(false)
    }
  }

  async function onAddComment(e: FormEvent) {
    e.preventDefault()
    if (!id) return
    setSubmittingComment(true)
    setError(null)
    try {
      await api.addComment(id, { author: commentAuthor, body: commentBody, internal: commentInternal })
      setCommentBody('')
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSubmittingComment(false)
    }
  }

  if (loading) return <div style={{ padding: 24 }}>Loading…</div>
  if (!ticket) return <div style={{ padding: 24, color: '#dc2626' }}>{error ?? 'Ticket not found'}</div>

  const nextStates = ALLOWED_TRANSITIONS[ticket.status]

  return (
    <div style={{ padding: 24, maxWidth: 720 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <h1 style={{ fontSize: 24, margin: 0 }}>{ticket.subject}</h1>
        <StatusBadge status={ticket.status} />
        <PriorityBadge priority={ticket.priority} />
      </div>
      <p style={{ color: '#6b7280', fontSize: 15 }}>
        {ticket.id} · {ticket.customerEmail} · created {new Date(ticket.createdAt).toLocaleString()}
      </p>
      <p>{ticket.description}</p>

      {error && <p style={{ color: '#dc2626' }}>{error}</p>}

      <div style={{ margin: '16px 0' }}>
        <strong style={{ fontSize: 15, color: '#6b7280' }}>Move to:</strong>{' '}
        {nextStates.length === 0 && <span style={{ color: '#6b7280' }}>(terminal state)</span>}
        {nextStates.map((s) => (
          <button
            key={s}
            disabled={statusBusy}
            onClick={() => onChangeStatus(s)}
            style={{ marginRight: 8, padding: '4px 10px' }}
          >
            {ticket.status === 'resolved' && s === 'open' ? 'reopen' : s.replace('_', ' ')}
          </button>
        ))}
      </div>

      <h2 style={{ fontSize: 18 }}>Comments</h2>
      {ticket.comments.length === 0 && <p style={{ color: '#6b7280' }}>No comments yet.</p>}
      <ul style={{ listStyle: 'none', padding: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
        {ticket.comments.map((c) => (
          <li
            key={c.id}
            style={{
              padding: 10,
              borderRadius: 8,
              backgroundColor: c.internal ? '#fef3c7' : '#f3f4f6',
            }}
          >
            <div style={{ fontSize: 15, color: '#6b7280', display: 'flex', gap: 8 }}>
              <strong>{c.author}</strong>
              <span>{new Date(c.at).toLocaleString()}</span>
              {c.internal && <span style={{ fontWeight: 700 }}>internal</span>}
            </div>
            <div>{c.body}</div>
          </li>
        ))}
      </ul>

      <form onSubmit={onAddComment} style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 8 }}>
        <h3 style={{ fontSize: 16, marginBottom: 0 }}>Add comment</h3>
        <input
          placeholder="Your name"
          value={commentAuthor}
          onChange={(e) => setCommentAuthor(e.target.value)}
          style={{ padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6 }}
        />
        <textarea
          placeholder="Comment body"
          value={commentBody}
          onChange={(e) => setCommentBody(e.target.value)}
          rows={3}
          style={{ padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6 }}
        />
        <label style={{ fontSize: 15 }}>
          <input
            type="checkbox"
            checked={commentInternal}
            onChange={(e) => setCommentInternal(e.target.checked)}
          />{' '}
          Internal note (never shown to the customer)
        </label>
        <button type="submit" disabled={submittingComment} style={{ padding: '8px 16px', width: 160 }}>
          {submittingComment ? 'Posting…' : 'Post comment'}
        </button>
      </form>
    </div>
  )
}
