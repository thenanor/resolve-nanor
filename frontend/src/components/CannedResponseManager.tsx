import { useState } from 'react'
import type { CannedResponse } from './cannedResponseTypes'

export type { CannedResponse }

export interface CannedResponseSaveFields {
  title: string
  body: string
  tags: string[]
}

interface CannedResponseManagerProps {
  cannedResponses: CannedResponse[]
  onDelete: (id: string) => void
  onSave: (id: string, fields: CannedResponseSaveFields) => void
}

function parseTags(value: string): string[] {
  return value
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '')
}

// Delete (AC-24) and save-edit (AC-25) both require a confirmation prompt
// naming/describing the affected canned response; canceling is a true
// no-op — onDelete/onSave are only called after the prompt is accepted.
export function CannedResponseManager({ cannedResponses, onDelete, onSave }: CannedResponseManagerProps) {
  const [editingId, setEditingId] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [tags, setTags] = useState('')

  function startEdit(cr: CannedResponse) {
    setEditingId(cr.id)
    setTitle(cr.title)
    setBody(cr.body)
    setTags(cr.tags.join(', '))
  }

  function cancelEdit() {
    setEditingId(null)
  }

  function saveEdit(cr: CannedResponse) {
    if (!window.confirm(`Save changes to "${cr.title}"?`)) return
    onSave(cr.id, { title, body, tags: parseTags(tags) })
    setEditingId(null)
  }

  function deleteResponse(cr: CannedResponse) {
    if (!window.confirm(`Delete canned response "${cr.title}"? This cannot be undone.`)) return
    onDelete(cr.id)
  }

  return (
    <ul style={{ listStyle: 'none', padding: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
      {cannedResponses.map((cr) =>
        editingId === cr.id ? (
          <li key={cr.id} style={{ padding: 10, border: '1px solid #d1d5db', borderRadius: 8 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <label htmlFor={`cr-title-${cr.id}`}>Title</label>
              <input id={`cr-title-${cr.id}`} value={title} onChange={(e) => setTitle(e.target.value)} />

              <label htmlFor={`cr-body-${cr.id}`}>Body</label>
              <textarea id={`cr-body-${cr.id}`} value={body} onChange={(e) => setBody(e.target.value)} rows={3} />

              <label htmlFor={`cr-tags-${cr.id}`}>Tags (comma-separated)</label>
              <input id={`cr-tags-${cr.id}`} value={tags} onChange={(e) => setTags(e.target.value)} />

              <div style={{ display: 'flex', gap: 8 }}>
                <button type="button" onClick={() => saveEdit(cr)}>
                  Save
                </button>
                <button type="button" onClick={cancelEdit}>
                  Cancel
                </button>
              </div>
            </div>
          </li>
        ) : (
          <li
            key={cr.id}
            style={{
              padding: 10,
              border: '1px solid #e5e7eb',
              borderRadius: 8,
              display: 'flex',
              alignItems: 'center',
              gap: 12,
            }}
          >
            <div style={{ flex: 1 }}>
              <strong>{cr.title}</strong>
              <div style={{ color: '#6b7280', fontSize: 15 }}>{cr.tags.join(', ')}</div>
            </div>
            <button type="button" onClick={() => startEdit(cr)}>
              Edit
            </button>
            <button type="button" onClick={() => deleteResponse(cr)}>
              Delete
            </button>
          </li>
        ),
      )}
    </ul>
  )
}
