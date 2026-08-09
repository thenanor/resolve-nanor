package cannedresponses

import (
	"context"
	"strings"
	"time"

	"resolve/internal/ids"
)

// CreateInput is also used by Update — both accept the same shape (title,
// body, tags) and apply the same normalization rules (AC-5/AC-6/AC-16).
type CreateInput struct {
	Title string
	Body  string
	Tags  []string
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Service) Create(ctx context.Context, input CreateInput) (*CannedResponse, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, invalid("title must be a non-empty string")
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return nil, invalid("body must be a non-empty string")
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return nil, err
	}

	ts := now()
	cr := &CannedResponse{
		ID:        ids.New("cr"),
		Title:     title,
		Body:      body,
		Tags:      tags,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	if err := s.repo.Create(ctx, cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) Update(ctx context.Context, id string, input CreateInput) (*CannedResponse, error) {
	cr, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, invalid("title must be a non-empty string")
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return nil, invalid("body must be a non-empty string")
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return nil, err
	}

	cr.Title = title
	cr.Body = body
	cr.Tags = tags
	cr.UpdatedAt = now()

	if err := s.repo.Update(ctx, cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) FindAll(ctx context.Context, filter Filter) ([]CannedResponse, error) {
	return s.repo.FindAll(ctx, filter)
}

func (s *Service) FindByID(ctx context.Context, id string) (*CannedResponse, error) {
	cr, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cr == nil {
		return nil, &NotFoundError{ID: id}
	}
	return cr, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// normalizeTags trims and lower-cases each tag (AC-5), rejects any entry
// that is empty after trimming (AC-6), and defaults a nil input to an empty
// (never nil) slice (AC-4) so it serializes as [] rather than null.
func normalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		trimmed := strings.ToLower(strings.TrimSpace(t))
		if trimmed == "" {
			return nil, invalid("tags must not contain an empty or whitespace-only entry")
		}
		out = append(out, trimmed)
	}
	return out, nil
}
