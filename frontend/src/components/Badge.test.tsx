import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PriorityBadge, StatusBadge } from './Badge'

describe('StatusBadge', () => {
  it('renders underscored statuses with a space', () => {
    render(<StatusBadge status="in_progress" />)
    expect(screen.getByText('in progress')).toBeInTheDocument()
  })

  it('renders single-word statuses as-is', () => {
    render(<StatusBadge status="closed" />)
    expect(screen.getByText('closed')).toBeInTheDocument()
  })
})

describe('PriorityBadge', () => {
  it('renders the priority label', () => {
    render(<PriorityBadge priority="urgent" />)
    expect(screen.getByText('urgent')).toBeInTheDocument()
  })
})
