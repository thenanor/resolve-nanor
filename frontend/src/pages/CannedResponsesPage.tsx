import { useCallback, useEffect, useState, type CSSProperties, type FormEvent } from 'react'
import { api } from '../api/client'
import { CannedResponseManager } from '../components/CannedResponseManager'
import type { CannedResponse } from '../types'

export function CannedResponsesPage() {
  const [responses, setResponses] = useState<CannedResponse[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [tags, setTags] = useState('')
  const [creating, setCreating] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    api
      .listCannedResponses()
      .then(setResponses)
      .catch((e) => setError((e as Error).message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError(null)
    try {
      await api.createCannedResponse({
        title,
        body,
        tags: tags
          .split(',')
          .map((t) => t.trim())
          .filter((t) => t !== ''),
      })
      setTitle('')
      setBody('')
      setTags('')
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setCreating(false)
    }
  }

  async function onSave(id: string, fields: { title: string; body: string; tags: string[] }) {
    setError(null)
    try {
      await api.updateCannedResponse(id, fields)
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function onDelete(id: string) {
    setError(null)
    try {
      await api.deleteCannedResponse(id)
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 720 }}>
      <h1 style={{ fontSize: 24 }}>Canned Responses</h1>

      {error && <p style={{ color: '#dc2626' }}>{error}</p>}

      <form onSubmit={onCreate} style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 24 }}>
        <h2 style={{ fontSize: 16, marginBottom: 0 }}>New canned response</h2>
        <label>
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} style={input} />
        </label>
        <label>
          Body
          <textarea value={body} onChange={(e) => setBody(e.target.value)} rows={3} style={input} />
        </label>
        <label>
          Tags (comma-separated)
          <input value={tags} onChange={(e) => setTags(e.target.value)} style={input} />
        </label>
        <button type="submit" disabled={creating} style={{ padding: '8px 16px', width: 160 }}>
          {creating ? 'Creating…' : 'Create'}
        </button>
      </form>

      <h2 style={{ fontSize: 16 }}>Existing canned responses</h2>
      {loading && <p style={{ color: '#6b7280' }}>Loading…</p>}
      {!loading && responses.length === 0 && <p style={{ color: '#6b7280' }}>No canned responses yet.</p>}
      <CannedResponseManager cannedResponses={responses} onDelete={onDelete} onSave={onSave} />
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
