package cannedresponses_test

// TestAC19_DeletingCannedResponseDoesNotAlterExistingComments specifies
// AC-19 and the related Invariant from .claude/specs/canned-responses.md:
// deleting a canned response never mutates a ticket comment that was
// previously seeded from its text, because no stored reference connects
// ticket_comments back to canned_responses. It spans both the existing
// tickets package and the not-yet-implemented cannedresponses package, so
// it lives in an external test package rather than inside handler_test.go.

import (
	"context"
	"errors"
	"testing"

	"resolve/internal/cannedresponses"
	"resolve/internal/tickets"
)

type fakeTicketRepository struct {
	byID map[string]*tickets.Ticket
}

func newFakeTicketRepository() *fakeTicketRepository {
	return &fakeTicketRepository{byID: map[string]*tickets.Ticket{}}
}

func (f *fakeTicketRepository) CreateTicket(_ context.Context, t *tickets.Ticket) error {
	cp := *t
	f.byID[t.ID] = &cp
	return nil
}

func (f *fakeTicketRepository) FindAll(_ context.Context, _ tickets.Filter) ([]tickets.Ticket, error) {
	return nil, nil
}

func (f *fakeTicketRepository) FindPage(_ context.Context, _ tickets.Filter, _ tickets.Pagination) (tickets.Page, error) {
	return tickets.Page{}, nil
}

func (f *fakeTicketRepository) FindByID(_ context.Context, id string) (*tickets.Ticket, error) {
	t, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	cp.Comments = append([]tickets.Comment{}, t.Comments...)
	return &cp, nil
}

func (f *fakeTicketRepository) UpdateTicket(_ context.Context, t *tickets.Ticket) error {
	if _, ok := f.byID[t.ID]; !ok {
		return errors.New("not found")
	}
	cp := *t
	f.byID[t.ID] = &cp
	return nil
}

func (f *fakeTicketRepository) AddComment(_ context.Context, ticketID string, c tickets.Comment) error {
	t, ok := f.byID[ticketID]
	if !ok {
		return errors.New("not found")
	}
	t.Comments = append(t.Comments, c)
	return nil
}

type fakeAuditRecorder struct{}

func (fakeAuditRecorder) Record(_ context.Context, _, _, _ string, _ map[string]any) error {
	return nil
}

func (fakeAuditRecorder) List(_ context.Context, _ string) ([]tickets.AuditEntry, error) {
	return nil, nil
}

// fakeCannedResponseRepository duplicates the shape of the fake in
// handler_test.go — that one lives in package cannedresponses (internal
// tests) and is not visible from this external test package.
type fakeCannedResponseRepository struct {
	byID map[string]*cannedresponses.CannedResponse
}

func newFakeCannedResponseRepository() *fakeCannedResponseRepository {
	return &fakeCannedResponseRepository{byID: map[string]*cannedresponses.CannedResponse{}}
}

func (f *fakeCannedResponseRepository) Create(_ context.Context, cr *cannedresponses.CannedResponse) error {
	cp := *cr
	f.byID[cr.ID] = &cp
	return nil
}

func (f *fakeCannedResponseRepository) FindAll(_ context.Context, _ cannedresponses.Filter) ([]cannedresponses.CannedResponse, error) {
	var out []cannedresponses.CannedResponse
	for _, cr := range f.byID {
		out = append(out, *cr)
	}
	return out, nil
}

func (f *fakeCannedResponseRepository) FindByID(_ context.Context, id string) (*cannedresponses.CannedResponse, error) {
	cr, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *cr
	return &cp, nil
}

func (f *fakeCannedResponseRepository) Update(_ context.Context, cr *cannedresponses.CannedResponse) error {
	if _, ok := f.byID[cr.ID]; !ok {
		return errors.New("not found")
	}
	cp := *cr
	f.byID[cr.ID] = &cp
	return nil
}

func (f *fakeCannedResponseRepository) Delete(_ context.Context, id string) error {
	if _, ok := f.byID[id]; !ok {
		return errors.New("not found")
	}
	delete(f.byID, id)
	return nil
}

func TestAC19_DeletingCannedResponseDoesNotAlterExistingComments(t *testing.T) {
	ctx := context.Background()

	ticketSvc := tickets.NewService(newFakeTicketRepository(), fakeAuditRecorder{})
	ticket, err := ticketSvc.Create(ctx, "agent-1", tickets.CreateInput{
		Subject:       "Cannot log in",
		Description:   "Password reset email never arrives",
		CustomerEmail: "ani@example.am",
		Priority:      "high",
	})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	crSvc := cannedresponses.NewService(newFakeCannedResponseRepository())
	cr, err := crSvc.Create(ctx, cannedresponses.CreateInput{
		Title: "Password reset",
		Body:  "Please reset your password via the link we just emailed you.",
	})
	if err != nil {
		t.Fatalf("create canned response: %v", err)
	}

	comment, err := ticketSvc.AddComment(ctx, "agent-1", ticket.ID, tickets.CommentInput{
		Author: "agent-1",
		Body:   cr.Body,
	})
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}

	if err := crSvc.Delete(ctx, cr.ID); err != nil {
		t.Fatalf("delete canned response: %v", err)
	}

	final, err := ticketSvc.FindByID(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("find ticket: %v", err)
	}
	if len(final.Comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(final.Comments))
	}
	if final.Comments[0].Body != comment.Body {
		t.Errorf("comment body = %q, want unchanged %q", final.Comments[0].Body, comment.Body)
	}
}
