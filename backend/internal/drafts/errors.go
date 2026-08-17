package drafts

import "fmt"

// ValidationError mirrors tickets.ValidationError — the message names the
// offending field so callers can react to it directly.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// NotFoundError mirrors tickets.NotFoundError.
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("draft %s not found", e.ID) }

// ConflictError is returned when a draft's current state doesn't allow the
// requested operation (e.g. sending a draft that isn't cleanly guarded).
// tickets/errors.go has no equivalent — nothing in that package has a
// multi-step lifecycle a request can conflict with.
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

func conflict(format string, args ...any) error {
	return &ConflictError{Message: fmt.Sprintf(format, args...)}
}
