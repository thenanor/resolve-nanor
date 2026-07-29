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
