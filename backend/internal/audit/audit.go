package audit

import (
	"context"
	"time"
)

type Entry struct {
	Seq      int64          `json:"-"`
	Actor    string         `json:"actor"`
	Action   string         `json:"action"`
	TicketID string         `json:"ticketId"`
	Details  map[string]any `json:"details"`
	At       string         `json:"at"`
}

// Repository is the persistence boundary for audit entries.
type Repository interface {
	Insert(ctx context.Context, e Entry) error
	List(ctx context.Context, ticketID string) ([]Entry, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Record(ctx context.Context, actor, action, ticketID string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	return s.repo.Insert(ctx, Entry{
		Actor:    actor,
		Action:   action,
		TicketID: ticketID,
		Details:  details,
		At:       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Service) List(ctx context.Context, ticketID string) ([]Entry, error) {
	return s.repo.List(ctx, ticketID)
}
