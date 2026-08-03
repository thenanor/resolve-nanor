package tickets

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// seedTickets creates n tickets via the service (so they go through normal
// validation and audit recording) but overwrites CreatedAt with a strictly
// increasing synthetic timestamp, so page order is deterministic regardless
// of clock resolution. withInput lets callers vary fields (e.g. priority)
// across the batch.
func seedTickets(t *testing.T, svc *Service, repo *fakeRepository, n int, withInput func(i int, in CreateInput) CreateInput) []*Ticket {
	t.Helper()
	ctx := context.Background()
	tickets := make([]*Ticket, n)
	for i := 0; i < n; i++ {
		in := validInput
		if withInput != nil {
			in = withInput(i, in)
		}
		ticket, err := svc.Create(ctx, "test", in)
		if err != nil {
			t.Fatalf("seed ticket %d: %v", i, err)
		}
		stored := repo.byID[ticket.ID]
		stored.CreatedAt = fmt.Sprintf("2024-01-01T00:00:%02dZ", i)
		ticket.CreatedAt = stored.CreatedAt
		tickets[i] = ticket
	}
	return tickets
}

func newPaginationFixture() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	return svc, repo
}

func TestFindPage_DefaultsLimitTo50(t *testing.T) {
	svc, repo := newPaginationFixture()
	seedTickets(t, svc, repo, 3, nil)

	page, err := svc.FindPage(context.Background(), Filter{}, Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Limit != DefaultPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, DefaultPageLimit)
	}
	if len(page.Tickets) != 3 {
		t.Errorf("len(tickets) = %d, want 3", len(page.Tickets))
	}
	if page.HasMore {
		t.Error("hasMore = true, want false")
	}
}

func TestFindPage_ClampsLimitToMax(t *testing.T) {
	svc, repo := newPaginationFixture()
	seedTickets(t, svc, repo, 2, nil)

	page, err := svc.FindPage(context.Background(), Filter{}, Pagination{Limit: 5000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Limit != MaxPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, MaxPageLimit)
	}
}

func TestFindPage_RejectsNegativeLimit(t *testing.T) {
	svc, _ := newPaginationFixture()

	_, err := svc.FindPage(context.Background(), Filter{}, Pagination{Limit: -1})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestFindPage_RejectsNegativeOffset(t *testing.T) {
	svc, _ := newPaginationFixture()

	_, err := svc.FindPage(context.Background(), Filter{}, Pagination{Offset: -1})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestFindPage_OrdersByCreationAscending(t *testing.T) {
	svc, repo := newPaginationFixture()
	seeded := seedTickets(t, svc, repo, 5, nil)

	page, err := svc.FindPage(context.Background(), Filter{}, Pagination{Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, want := range seeded {
		if page.Tickets[i].ID != want.ID {
			t.Errorf("tickets[%d].ID = %q, want %q", i, page.Tickets[i].ID, want.ID)
		}
	}
}

func TestFindPage_HasMoreReflectsRemainingRows(t *testing.T) {
	svc, repo := newPaginationFixture()
	seedTickets(t, svc, repo, 5, nil)
	ctx := context.Background()

	page, err := svc.FindPage(ctx, Filter{}, Pagination{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Tickets) != 2 || !page.HasMore {
		t.Errorf("len=%d hasMore=%v, want len=2 hasMore=true", len(page.Tickets), page.HasMore)
	}

	page, err = svc.FindPage(ctx, Filter{}, Pagination{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Tickets) != 1 || page.HasMore {
		t.Errorf("len=%d hasMore=%v, want len=1 hasMore=false", len(page.Tickets), page.HasMore)
	}

	page, err = svc.FindPage(ctx, Filter{}, Pagination{Limit: 5, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Tickets) != 5 || page.HasMore {
		t.Errorf("len=%d hasMore=%v, want len=5 hasMore=false (exact boundary)", len(page.Tickets), page.HasMore)
	}
}

func TestFindPage_OffsetAppliesAfterFiltering(t *testing.T) {
	svc, repo := newPaginationFixture()
	// 5 low-priority tickets, then 3 high-priority tickets, all interleaved
	// in creation order so offset must be computed against the *filtered*
	// set, not the raw table.
	seeded := seedTickets(t, svc, repo, 8, func(i int, in CreateInput) CreateInput {
		if i < 5 {
			return withPriority(in, "low")
		}
		return withPriority(in, "high")
	})
	wantHighIDs := []string{seeded[5].ID, seeded[6].ID, seeded[7].ID}

	page, err := svc.FindPage(context.Background(), Filter{Priority: PriorityHigh}, Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Tickets) != 2 {
		t.Fatalf("len(tickets) = %d, want 2", len(page.Tickets))
	}
	if page.Tickets[0].ID != wantHighIDs[1] || page.Tickets[1].ID != wantHighIDs[2] {
		t.Errorf("got IDs [%s, %s], want [%s, %s]",
			page.Tickets[0].ID, page.Tickets[1].ID, wantHighIDs[1], wantHighIDs[2])
	}
	if page.HasMore {
		t.Error("hasMore = true, want false")
	}
}

func TestFindPage_OffsetPastEndReturnsEmptyPage(t *testing.T) {
	svc, repo := newPaginationFixture()
	seedTickets(t, svc, repo, 3, nil)

	page, err := svc.FindPage(context.Background(), Filter{}, Pagination{Limit: 50, Offset: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Tickets) != 0 {
		t.Errorf("len(tickets) = %d, want 0", len(page.Tickets))
	}
	if page.HasMore {
		t.Error("hasMore = true, want false")
	}
}
