import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { AuditEntry } from '../types'

export function AuditLogPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const ticketId = searchParams.get('ticketId') ?? ''
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    api
      .listAudit(ticketId || undefined)
      .then(setEntries)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [ticketId])

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
        <h1 style={{ fontSize: 20, margin: 0 }}>Audit Log</h1>
        <input
          placeholder="Filter by ticket id"
          value={ticketId}
          onChange={(e) => {
            const v = e.target.value
            setSearchParams(v ? { ticketId: v } : {})
          }}
          style={{ padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6 }}
        />
      </div>

      {error && <p style={{ color: '#dc2626' }}>{error}</p>}
      {loading ? (
        <p>Loading…</p>
      ) : entries.length === 0 ? (
        <p style={{ color: '#6b7280' }}>No audit entries found.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ textAlign: 'left', borderBottom: '1px solid #e5e7eb' }}>
              <th style={th}>When</th>
              <th style={th}>Actor</th>
              <th style={th}>Action</th>
              <th style={th}>Ticket</th>
              <th style={th}>Details</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={i} style={{ borderBottom: '1px solid #f3f4f6' }}>
                <td style={td}>{new Date(e.at).toLocaleString()}</td>
                <td style={td}>{e.actor}</td>
                <td style={td}>{e.action}</td>
                <td style={td}>
                  <Link to={`/tickets/${e.ticketId}`}>{e.ticketId}</Link>
                </td>
                <td style={{ ...td, fontFamily: 'monospace', fontSize: 12 }}>
                  {JSON.stringify(e.details)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

const th = { padding: '8px 4px', fontSize: 13, color: '#6b7280' }
const td = { padding: '8px 4px', fontSize: 14 }
