package triage

import (
	"context"
	"errors"
	"testing"
)

type fakeClassifier struct {
	result Result
	err    error
}

func (f fakeClassifier) Classify(context.Context, string, string) (Result, error) {
	return f.result, f.err
}

type updateCall struct {
	ticketID, category, priority, confidence string
}

type fakeTicketUpdater struct {
	calls []updateCall
	err   error
}

func (f *fakeTicketUpdater) UpdateTriage(_ context.Context, ticketID, category, priority, confidence string) error {
	f.calls = append(f.calls, updateCall{ticketID, category, priority, confidence})
	return f.err
}

func TestService_Triage_HappyPath(t *testing.T) {
	classifier := fakeClassifier{result: Result{Category: "billing", Priority: "urgent", Confidence: 0.9}}
	updater := &fakeTicketUpdater{}
	svc := NewService(classifier, updater)

	if err := svc.Triage(context.Background(), "tkt_1", "subject", "description"); err != nil {
		t.Fatalf("Triage returned error: %v", err)
	}

	if len(updater.calls) != 1 {
		t.Fatalf("updater called %d times, want 1", len(updater.calls))
	}
	got := updater.calls[0]
	want := updateCall{"tkt_1", "billing", "urgent", "high"}
	if got != want {
		t.Errorf("update call = %+v, want %+v", got, want)
	}
}

func TestService_Triage_ClassifierErrorShortCircuits(t *testing.T) {
	classifier := fakeClassifier{err: errors.New("anthropic unavailable")}
	updater := &fakeTicketUpdater{}
	svc := NewService(classifier, updater)

	err := svc.Triage(context.Background(), "tkt_1", "subject", "description")
	if err == nil {
		t.Fatal("expected an error when the classifier fails")
	}
	if len(updater.calls) != 0 {
		t.Errorf("expected updater not to be called, got %d calls", len(updater.calls))
	}
}

func TestService_Triage_UpdaterErrorIsSurfaced(t *testing.T) {
	classifier := fakeClassifier{result: Result{Category: "bug", Priority: "low", Confidence: 0.5}}
	updater := &fakeTicketUpdater{err: errors.New("main app unreachable")}
	svc := NewService(classifier, updater)

	if err := svc.Triage(context.Background(), "tkt_1", "subject", "description"); err == nil {
		t.Fatal("expected the updater's error to be surfaced, not swallowed")
	}
}
