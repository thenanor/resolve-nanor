import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { api, ApiRequestError } from '../api/client'
import { CategoryBadge, NeedsReviewBadge, PriorityBadge, StatusBadge } from '../components/Badge'
import { CannedResponsePicker } from '../components/CannedResponsePicker'
import { ALLOWED_TRANSITIONS } from '../lib/transitions'
import type { CannedResponse, GuardRejection, Ticket } from '../types'

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
  const [cannedResponses, setCannedResponses] = useState<CannedResponse[]>([])
  const [reviewBusy, setReviewBusy] = useState(false)

  // REPLYGUARD-1 AC-23: tracks whether commentBody is still exactly the
  // text a canned response was inserted as — cleared on any edit — so
  // onAddComment can tell the backend to skip the guard for it.
  const [insertedCannedBody, setInsertedCannedBody] = useState<string | null>(null)
  // A 409 guard rejection (verdict revise or escalate) for the in-flight
  // comment, with its findings — AC-24/AC-25.
  const [guardRejection, setGuardRejection] = useState<GuardRejection | null>(null)
  const [overrideReason, setOverrideReason] = useState('')

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

  useEffect(() => {
    api.listCannedResponses().then(setCannedResponses).catch(() => setCannedResponses([]))
  }, [])

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

  async function onReviewTriage(decision: 'accept' | 'reject') {
    if (!id) return
    setReviewBusy(true)
    setError(null)
    try {
      const updated = await api.reviewTriage(id, decision)
      setTicket(updated)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setReviewBusy(false)
    }
  }

  async function onAddComment(e: FormEvent) {
    e.preventDefault()
    if (!id) return
    setSubmittingComment(true)
    setError(null)
    setGuardRejection(null)
    try {
      await api.addComment(id, {
        author: commentAuthor,
        body: commentBody,
        internal: commentInternal,
        fromCannedResponse: insertedCannedBody !== null && insertedCannedBody === commentBody,
      })
      setCommentBody('')
      setInsertedCannedBody(null)
      setOverrideReason('')
      load()
    } catch (e) {
      if (e instanceof ApiRequestError && e.status === 409) {
        setGuardRejection(e.body as GuardRejection)
      } else {
        setError((e as Error).message)
      }
    } finally {
      setSubmittingComment(false)
    }
  }

  // AC-19: resubmits the same reply with overrideReason, waiving a
  // "revise" verdict. Never offered for "escalate" (AC-25).
  async function onSendAnyway() {
    if (!id || !overrideReason.trim()) return
    setSubmittingComment(true)
    setError(null)
    try {
      await api.addComment(id, {
        author: commentAuthor,
        body: commentBody,
        internal: commentInternal,
        overrideReason,
      })
      setCommentBody('')
      setInsertedCannedBody(null)
      setOverrideReason('')
      setGuardRejection(null)
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSubmittingComment(false)
    }
  }

  function onInsertCanned(body: string) {
    setCommentBody(body)
    setInsertedCannedBody(body)
  }

  function onEditCommentBody(value: string) {
    setCommentBody(value)
    setInsertedCannedBody(null)
    setGuardRejection(null)
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
        {ticket.category && <CategoryBadge category={ticket.category} />}
        {ticket.pendingCategory && <NeedsReviewBadge />}
      </div>
      <p style={{ color: '#6b7280', fontSize: 15 }}>
        {ticket.id} · {ticket.customerEmail} · created {new Date(ticket.createdAt).toLocaleString()}
      </p>
      <p>{ticket.description}</p>

      {error && <p style={{ color: '#dc2626' }}>{error}</p>}

      {ticket.pendingCategory && ticket.pendingPriority && (
        <div
          style={{
            margin: '16px 0',
            padding: 12,
            borderRadius: 8,
            backgroundColor: '#fffbeb',
            border: '1px solid #fde68a',
          }}
        >
          <strong style={{ fontSize: 15 }}>AI suggests: </strong>
          <CategoryBadge category={ticket.pendingCategory} /> <PriorityBadge priority={ticket.pendingPriority} />
          <div style={{ marginTop: 8 }}>
            <button disabled={reviewBusy} onClick={() => onReviewTriage('accept')} style={{ marginRight: 8, padding: '4px 10px' }}>
              Accept
            </button>
            <button disabled={reviewBusy} onClick={() => onReviewTriage('reject')} style={{ padding: '4px 10px' }}>
              Reject
            </button>
          </div>
        </div>
      )}

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
            {s.replace('_', ' ')}
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
        {cannedResponses.length > 0 && (
          <CannedResponsePicker cannedResponses={cannedResponses} onInsert={onInsertCanned} />
        )}
        <input
          placeholder="Your name"
          value={commentAuthor}
          onChange={(e) => setCommentAuthor(e.target.value)}
          style={{ padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6 }}
        />
        <textarea
          placeholder="Comment body"
          value={commentBody}
          onChange={(e) => onEditCommentBody(e.target.value)}
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

        {guardRejection && (
          <div
            style={{
              padding: 12,
              borderRadius: 8,
              backgroundColor: '#fef2f2',
              border: '1px solid #fecaca',
            }}
          >
            <strong style={{ fontSize: 15 }}>
              Reply-guard {guardRejection.verdict === 'escalate' ? 'blocked this reply' : 'flagged this reply'}
            </strong>
            <p style={{ fontSize: 14, color: '#6b7280', margin: '4px 0' }}>{guardRejection.reasoning}</p>
            <ul style={{ paddingLeft: 20, margin: '8px 0' }}>
              {guardRejection.findings.map((f, i) => (
                <li key={i} style={{ fontSize: 14, marginBottom: 4 }}>
                  <strong>
                    {f.policy} ({f.severity})
                  </strong>
                  : {f.issue} — <em>&ldquo;{f.quote}&rdquo;</em>
                </li>
              ))}
            </ul>
            {guardRejection.verdict === 'revise' ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <input
                  placeholder="Reason for sending anyway"
                  value={overrideReason}
                  onChange={(e) => setOverrideReason(e.target.value)}
                  style={{ padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6 }}
                />
                <button
                  type="button"
                  disabled={submittingComment || !overrideReason.trim()}
                  onClick={onSendAnyway}
                  style={{ padding: '6px 12px', width: 160 }}
                >
                  Send anyway
                </button>
              </div>
            ) : (
              <p style={{ fontSize: 14, color: '#6b7280', margin: 0 }}>
                This verdict can&apos;t be overridden — edit the reply and send it again.
              </p>
            )}
          </div>
        )}
      </form>
    </div>
  )
}
