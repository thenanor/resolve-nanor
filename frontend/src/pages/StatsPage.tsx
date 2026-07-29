import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Stats } from '../types'

export function StatsPage() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.getStats().then(setStats).catch((e) => setError(e.message))
  }, [])

  if (error) return <div style={{ padding: 24, color: '#dc2626' }}>{error}</div>
  if (!stats) return <div style={{ padding: 24 }}>Loading…</div>

  return (
    <div style={{ padding: 24, maxWidth: 640 }}>
      <h1 style={{ fontSize: 24 }}>Stats</h1>

      <div style={{ display: 'flex', gap: 16, marginBottom: 24 }}>
        <StatTile label="Total tickets" value={stats.total} />
        <StatTile
          label="Avg. resolution (min)"
          value={stats.avgResolutionMinutes ?? '—'}
        />
      </div>

      <div style={{ display: 'flex', gap: 32 }}>
        <div>
          <h2 style={{ fontSize: 16, color: '#6b7280' }}>By status</h2>
          <CountTable counts={stats.byStatus} />
        </div>
        <div>
          <h2 style={{ fontSize: 16, color: '#6b7280' }}>By priority</h2>
          <CountTable counts={stats.byPriority} />
        </div>
      </div>
    </div>
  )
}

function StatTile({ label, value }: { label: string; value: number | string }) {
  return (
    <div style={{ border: '1px solid #e5e7eb', borderRadius: 8, padding: 16, minWidth: 160 }}>
      <div style={{ fontSize: 15, color: '#6b7280' }}>{label}</div>
      <div style={{ fontSize: 32, fontWeight: 700 }}>{value}</div>
    </div>
  )
}

function CountTable({ counts }: { counts: Record<string, number> }) {
  const entries = Object.entries(counts)
  if (entries.length === 0) return <p style={{ color: '#6b7280' }}>No data.</p>
  return (
    <table style={{ borderCollapse: 'collapse' }}>
      <tbody>
        {entries.map(([key, count]) => (
          <tr key={key}>
            <td style={{ padding: '4px 12px 4px 0', fontSize: 16 }}>{key}</td>
            <td style={{ padding: '4px 0', fontSize: 16, fontWeight: 600 }}>{count}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
