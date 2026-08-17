package drafts

import "context"

// Repository is the persistence boundary for drafts, mirroring
// tickets.Repository's split of TypeORM's cascade-save into explicit,
// single-purpose operations.
type Repository interface {
	CreateDraft(ctx context.Context, d *Draft) error
	// FindByID returns nil, nil if no draft with this id exists. It is not
	// scoped by ticket id — Service checks Draft.TicketID against the URL's
	// ticket id itself, so a draft belonging to a different ticket is
	// reported the same way as a missing draft (see AC-25).
	FindByID(ctx context.Context, draftID string) (*Draft, error)
	// UpdateBody rewrites a draft's body, resets it to StatusPendingReview,
	// and clears any prior GuardResult, in one statement.
	UpdateBody(ctx context.Context, draftID, body, updatedAt string) error
	// SetGuardResult stores a completed guard assessment and moves the
	// draft to StatusGuarded.
	SetGuardResult(ctx context.Context, draftID string, result GuardResult, updatedAt string) error
	// SetGuardFailed moves the draft to StatusGuardFailed without touching
	// GuardResult, so a failed guard run is never mistaken for a clean one.
	SetGuardFailed(ctx context.Context, draftID, updatedAt string) error
	// MarkSent moves the draft to StatusSent.
	MarkSent(ctx context.Context, draftID, updatedAt string) error
}
