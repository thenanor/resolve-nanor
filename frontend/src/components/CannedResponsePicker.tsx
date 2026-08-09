import { useState } from 'react'
import type { CannedResponse } from './cannedResponseTypes'

export type { CannedResponse }

interface CannedResponsePickerProps {
  cannedResponses: CannedResponse[]
  onInsert: (body: string) => void
}

// Filters by a case-insensitive substring match on title and/or tag (AC-23).
// Inserting only calls onInsert with the body text — no fetch, no id, no
// author/internal fields (AC-20/AC-21/AC-22).
export function CannedResponsePicker({ cannedResponses, onInsert }: CannedResponsePickerProps) {
  const [query, setQuery] = useState('')

  const q = query.trim().toLowerCase()
  const filtered = q
    ? cannedResponses.filter(
        (cr) =>
          cr.title.toLowerCase().includes(q) || cr.tags.some((tag) => tag.toLowerCase().includes(q)),
      )
    : cannedResponses

  return (
    <div>
      <label htmlFor="canned-response-search" style={{ fontSize: 15, color: '#6b7280' }}>
        Search canned responses
      </label>
      <input
        id="canned-response-search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Filter by title or tag"
        style={{ display: 'block', padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 6, marginBottom: 8 }}
      />
      <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
        {filtered.map((cr) => (
          <li key={cr.id}>
            <button
              type="button"
              onClick={() => onInsert(cr.body)}
              style={{ padding: '4px 8px', textAlign: 'left', width: '100%' }}
            >
              {cr.title}
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
