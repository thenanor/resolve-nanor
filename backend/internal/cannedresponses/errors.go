package cannedresponses

import "fmt"

// ValidationError mirrors tickets.ValidationError — the message names the
// offending field so callers (and the frontend) can react to it directly.
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

func (e *NotFoundError) Error() string { return fmt.Sprintf("canned response %s not found", e.ID) }
