import type { TicketCategory, TicketPriority, TicketStatus } from '../types'

const STATUS_COLORS: Record<TicketStatus, string> = {
  new: '#6b7280',
  open: '#2563eb',
  in_progress: '#7c3aed',
  waiting_customer: '#d97706',
  resolved: '#059669',
  closed: '#111827',
}

const PRIORITY_COLORS: Record<TicketPriority, string> = {
  low: '#6b7280',
  normal: '#2563eb',
  high: '#d97706',
  urgent: '#dc2626',
}

const CATEGORY_COLORS: Record<TicketCategory, string> = {
  billing: '#7c3aed',
  bug: '#dc2626',
  account_access: '#d97706',
  feature_request: '#2563eb',
  how_to: '#059669',
  other: '#6b7280',
}

function Badge({ label, color }: { label: string; color: string }) {
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px 8px',
        borderRadius: 999,
        fontSize: 14,
        fontWeight: 600,
        color: '#fff',
        backgroundColor: color,
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </span>
  )
}

export function StatusBadge({ status }: { status: TicketStatus }) {
  return <Badge label={status.replace('_', ' ')} color={STATUS_COLORS[status]} />
}

export function PriorityBadge({ priority }: { priority: TicketPriority }) {
  return <Badge label={priority} color={PRIORITY_COLORS[priority]} />
}

export function CategoryBadge({ category }: { category: TicketCategory }) {
  return <Badge label={category.replace('_', ' ')} color={CATEGORY_COLORS[category]} />
}

export function NeedsReviewBadge() {
  return <Badge label="needs review" color="#b45309" />
}
