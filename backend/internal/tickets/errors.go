package tickets

import "fmt"

// ValidationError mirrors Nest's BadRequestException — the message names the
// offending field so callers (and the frontend) can react to it directly.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// NotFoundError mirrors Nest's NotFoundException(`ticket ${id} not found`).
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("ticket %s not found", e.ID) }

// GuardRejectedError is returned when reply-guard ran and rejected a
// candidate reply: a "revise" verdict with no (or empty) overrideReason,
// or an "escalate" verdict (which is never overridable). Distinct from
// GuardUnavailableError so the two "no Comment was created" failure modes
// stay distinguishable in code, not just by status code (AC-18/AC-20 vs
// AC-22).
type GuardRejectedError struct {
	Outcome ReplyGuardOutcome
}

func (e *GuardRejectedError) Error() string {
	return fmt.Sprintf("reply-guard rejected the candidate reply: verdict=%s", e.Outcome.Verdict)
}

// GuardUnavailableError is returned when the guard call itself could not
// be completed — reply-guard unreachable, non-2xx response, or
// malformed/invalid output (AC-22). Fails closed: AddComment never falls
// back to creating an unguarded comment when this is returned.
type GuardUnavailableError struct {
	Err error
}

func (e *GuardUnavailableError) Error() string {
	return fmt.Sprintf("reply-guard could not complete review: %v", e.Err)
}

func (e *GuardUnavailableError) Unwrap() error { return e.Err }
