import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { getActor, setActor } from '../api/client'

export function Nav() {
  const [actor, setActorState] = useState(getActor())

  function onActorChange(value: string) {
    setActorState(value)
    setActor(value)
  }

  const linkStyle = ({ isActive }: { isActive: boolean }) => ({
    fontWeight: isActive ? 700 : 400,
    color: isActive ? '#111827' : '#4b5563',
    textDecoration: 'none',
  })

  return (
    <nav
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 20,
        padding: '12px 24px',
        borderBottom: '1px solid #e5e7eb',
      }}
    >
      <strong style={{ marginRight: 8 }}>Resolve</strong>
      <NavLink to="/" style={linkStyle} end>
        Tickets
      </NavLink>
      <NavLink to="/tickets/new" style={linkStyle}>
        New Ticket
      </NavLink>
      <NavLink to="/audit" style={linkStyle}>
        Audit Log
      </NavLink>
      <NavLink to="/stats" style={linkStyle}>
        Stats
      </NavLink>
      <NavLink to="/canned-responses" style={linkStyle}>
        Canned Responses
      </NavLink>
      <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8 }}>
        <label htmlFor="actor" style={{ fontSize: 15, color: '#6b7280' }}>
          Acting as
        </label>
        <input
          id="actor"
          value={actor}
          onChange={(e) => onActorChange(e.target.value)}
          style={{ padding: '4px 8px', border: '1px solid #d1d5db', borderRadius: 6, width: 120 }}
        />
      </div>
    </nav>
  )
}
