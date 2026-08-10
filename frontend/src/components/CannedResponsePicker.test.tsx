// Specifies (does not implement) AC-20, AC-21, AC-22, AC-23 from
// .claude/specs/canned-responses.md. Imports a CannedResponsePicker
// component that does not exist yet, so this suite is expected to fail to
// resolve until that component is written.
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CannedResponsePicker, type CannedResponse } from './CannedResponsePicker'

const responses: CannedResponse[] = [
  {
    id: 'cr_aaaaaaaa',
    title: 'Password reset',
    body: 'Please reset your password via the link we just emailed you.',
    tags: ['billing', 'account'],
  },
  {
    id: 'cr_bbbbbbbb',
    title: 'Refund processed',
    body: 'Your refund has been processed and should arrive in 3-5 days.',
    tags: ['billing'],
  },
  {
    id: 'cr_cccccccc',
    title: 'Welcome',
    body: 'Welcome to Resolve support!',
    tags: [],
  },
]

describe('AC-20: inserting a canned response populates the comment body', () => {
  it('calls onInsert with the selected response\'s body text and nothing else', () => {
    const onInsert = vi.fn()
    render(<CannedResponsePicker cannedResponses={responses} onInsert={onInsert} />)

    fireEvent.click(screen.getByText('Password reset'))

    expect(onInsert).toHaveBeenCalledTimes(1)
    expect(onInsert).toHaveBeenCalledWith(
      'Please reset your password via the link we just emailed you.',
    )
  })

  it('does not call POST /tickets/{id}/comments itself and does not submit anything', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    render(<CannedResponsePicker cannedResponses={responses} onInsert={vi.fn()} />)

    fireEvent.click(screen.getByText('Welcome'))

    expect(fetchSpy).not.toHaveBeenCalled()
    fetchSpy.mockRestore()
  })
})

describe('AC-21 / AC-22: the picker only ever exposes body text, not author/internal/id', () => {
  it('onInsert receives a plain string, never an object carrying the canned response id', () => {
    const onInsert = vi.fn()
    render(<CannedResponsePicker cannedResponses={responses} onInsert={onInsert} />)

    fireEvent.click(screen.getByText('Refund processed'))

    const [arg] = onInsert.mock.calls[0] as [unknown]
    expect(typeof arg).toBe('string')
  })

  it('renders no author or internal form controls of its own', () => {
    render(<CannedResponsePicker cannedResponses={responses} onInsert={vi.fn()} />)

    expect(screen.queryByLabelText(/author/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/internal/i)).not.toBeInTheDocument()
  })
})

describe('AC-23: filtering the canned-response picker by tag and title', () => {
  it('filters by a case-insensitive substring match on title', () => {
    render(<CannedResponsePicker cannedResponses={responses} onInsert={vi.fn()} />)

    fireEvent.change(screen.getByLabelText(/search/i), { target: { value: 'REFUND' } })

    expect(screen.getByText('Refund processed')).toBeInTheDocument()
    expect(screen.queryByText('Password reset')).not.toBeInTheDocument()
    expect(screen.queryByText('Welcome')).not.toBeInTheDocument()
  })

  it('filters by tag', () => {
    render(<CannedResponsePicker cannedResponses={responses} onInsert={vi.fn()} />)

    fireEvent.change(screen.getByLabelText(/search/i), { target: { value: 'account' } })

    expect(screen.getByText('Password reset')).toBeInTheDocument()
    expect(screen.queryByText('Refund processed')).not.toBeInTheDocument()
    expect(screen.queryByText('Welcome')).not.toBeInTheDocument()
  })

  it('shows every response again once the filter is cleared', () => {
    render(<CannedResponsePicker cannedResponses={responses} onInsert={vi.fn()} />)

    const search = screen.getByLabelText(/search/i)
    fireEvent.change(search, { target: { value: 'billing' } })
    fireEvent.change(search, { target: { value: '' } })

    for (const r of responses) {
      expect(screen.getByText(r.title)).toBeInTheDocument()
    }
  })
})
