// Specifies (does not implement) AC-24 and AC-25 from
// .claude/specs/canned-responses.md. Imports a CannedResponseManager
// component that does not exist yet, so this suite is expected to fail to
// resolve until that component is written.
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CannedResponseManager, type CannedResponse } from './CannedResponseManager'

const responses: CannedResponse[] = [
  {
    id: 'cr_aaaaaaaa',
    title: 'Password reset',
    body: 'Please reset your password via the link we just emailed you.',
    tags: ['billing'],
  },
]

afterEach(() => {
  vi.restoreAllMocks()
})

describe('AC-24: deleting a canned response requires confirmation', () => {
  it('does not call onDelete when the confirmation prompt is dismissed', () => {
    const onDelete = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(
      <CannedResponseManager cannedResponses={responses} onDelete={onDelete} onSave={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    expect(window.confirm).toHaveBeenCalled()
    expect(onDelete).not.toHaveBeenCalled()
  })

  it('calls onDelete with the id when the confirmation prompt is accepted', () => {
    const onDelete = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(
      <CannedResponseManager cannedResponses={responses} onDelete={onDelete} onSave={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    expect(onDelete).toHaveBeenCalledWith('cr_aaaaaaaa')
  })

  it('names the canned response in the confirmation prompt', () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(
      <CannedResponseManager cannedResponses={responses} onDelete={vi.fn()} onSave={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('Password reset'))
  })
})

describe('AC-25: saving an edit to a canned response requires confirmation', () => {
  it('does not call onSave when the confirmation prompt is dismissed', () => {
    const onSave = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(
      <CannedResponseManager cannedResponses={responses} onDelete={vi.fn()} onSave={onSave} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))
    fireEvent.click(screen.getByRole('button', { name: /save/i }))

    expect(window.confirm).toHaveBeenCalled()
    expect(onSave).not.toHaveBeenCalled()
  })

  it('calls onSave with the updated fields when the confirmation prompt is accepted, and the stored fields are unchanged until then', () => {
    const onSave = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(
      <CannedResponseManager cannedResponses={responses} onDelete={vi.fn()} onSave={onSave} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))
    fireEvent.change(screen.getByLabelText(/title/i), {
      target: { value: 'Password reset (updated)' },
    })
    fireEvent.click(screen.getByRole('button', { name: /save/i }))

    expect(onSave).toHaveBeenCalledWith(
      'cr_aaaaaaaa',
      expect.objectContaining({ title: 'Password reset (updated)' }),
    )
  })
})
