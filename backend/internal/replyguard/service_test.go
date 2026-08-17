package replyguard

import (
	"context"
	"errors"
	"testing"

	"resolve/internal/drafts"
)

type fakeClassifier struct {
	result Result
	err    error
}

func (f fakeClassifier) Guard(context.Context, GuardInput) (Result, error) {
	return f.result, f.err
}

type updateCall struct {
	ticketID, draftID, verdict, confidence, reasoning string
	findings                                          []drafts.Finding
	injectionSuspected, requireHuman                  bool
}

type fakeDraftUpdater struct {
	calls []updateCall
	err   error
}

func (f *fakeDraftUpdater) UpdateGuardResult(_ context.Context, ticketID, draftID, verdict string, findings []drafts.Finding, confidence, reasoning string, injectionSuspected, requireHuman bool) error {
	f.calls = append(f.calls, updateCall{ticketID, draftID, verdict, confidence, reasoning, findings, injectionSuspected, requireHuman})
	return f.err
}

func TestService_Guard_HappyPath_CleanDraftSends(t *testing.T) {
	classifier := fakeClassifier{result: Result{Confidence: 0.9, Reasoning: "clean"}}
	updater := &fakeDraftUpdater{}
	svc := NewService(classifier, updater)

	if err := svc.Guard(context.Background(), "tkt_1", "dft_1", GuardInput{}); err != nil {
		t.Fatalf("Guard returned error: %v", err)
	}

	if len(updater.calls) != 1 {
		t.Fatalf("updater called %d times, want 1", len(updater.calls))
	}
	got := updater.calls[0]
	if got.ticketID != "tkt_1" || got.draftID != "dft_1" {
		t.Errorf("ids = %q/%q, want tkt_1/dft_1", got.ticketID, got.draftID)
	}
	if got.verdict != string(drafts.VerdictSend) {
		t.Errorf("verdict = %q, want %q", got.verdict, drafts.VerdictSend)
	}
	if got.confidence != "high" {
		t.Errorf("confidence = %q, want high", got.confidence)
	}
	if got.requireHuman {
		t.Errorf("requireHuman = true, want false for a clean, high-confidence send")
	}
}

func TestService_Guard_ClassifierErrorShortCircuits(t *testing.T) {
	classifier := fakeClassifier{err: errors.New("anthropic unavailable")}
	updater := &fakeDraftUpdater{}
	svc := NewService(classifier, updater)

	err := svc.Guard(context.Background(), "tkt_1", "dft_1", GuardInput{})
	if err == nil {
		t.Fatal("expected an error when the classifier fails")
	}
	if len(updater.calls) != 0 {
		t.Errorf("expected updater not to be called, got %d calls", len(updater.calls))
	}
}

func TestService_Guard_UpdaterErrorIsSurfaced(t *testing.T) {
	classifier := fakeClassifier{result: Result{Confidence: 0.9}}
	updater := &fakeDraftUpdater{err: errors.New("main app unreachable")}
	svc := NewService(classifier, updater)

	if err := svc.Guard(context.Background(), "tkt_1", "dft_1", GuardInput{}); err == nil {
		t.Fatal("expected the updater's error to be surfaced, not swallowed")
	}
}

// --- AC-13/14/15/16: verdict and requireHuman derivation ---

func TestDeriveVerdict_AC14_EmptyFindingsSends(t *testing.T) {
	if got := deriveVerdict(nil, false); got != drafts.VerdictSend {
		t.Errorf("deriveVerdict(nil, false) = %q, want send", got)
	}
}

func TestDeriveVerdict_AC14_OnlyLowSeverityFindingsSends(t *testing.T) {
	findings := []drafts.Finding{{Severity: drafts.SeverityLow}, {Severity: drafts.SeverityLow}}
	if got := deriveVerdict(findings, false); got != drafts.VerdictSend {
		t.Errorf("deriveVerdict(low-only, false) = %q, want send", got)
	}
}

func TestDeriveVerdict_AC15_AnyHighSeverityFindingEscalates(t *testing.T) {
	findings := []drafts.Finding{{Severity: drafts.SeverityLow}, {Severity: drafts.SeverityHigh}}
	if got := deriveVerdict(findings, false); got != drafts.VerdictEscalate {
		t.Errorf("deriveVerdict(with high, false) = %q, want escalate", got)
	}
}

func TestDeriveVerdict_AC15_InjectionSuspectedEscalatesEvenWithoutFindings(t *testing.T) {
	if got := deriveVerdict(nil, true); got != drafts.VerdictEscalate {
		t.Errorf("deriveVerdict(nil, true) = %q, want escalate", got)
	}
}

func TestDeriveVerdict_AC16_MediumSeverityWithNoHighOrInjectionRevises(t *testing.T) {
	findings := []drafts.Finding{{Severity: drafts.SeverityLow}, {Severity: drafts.SeverityMedium}}
	if got := deriveVerdict(findings, false); got != drafts.VerdictRevise {
		t.Errorf("deriveVerdict(medium, false) = %q, want revise", got)
	}
}

func TestDeriveRequireHuman_AC13_TrueOnEscalate(t *testing.T) {
	if !deriveRequireHuman(drafts.VerdictEscalate, nil, "high") {
		t.Error("expected requireHuman = true for an escalate verdict")
	}
}

func TestDeriveRequireHuman_AC13_TrueOnLowConfidenceEvenForSend(t *testing.T) {
	if !deriveRequireHuman(drafts.VerdictSend, nil, "low") {
		t.Error("expected requireHuman = true for a send verdict at low confidence")
	}
}

func TestDeriveRequireHuman_AC13_TrueOnAnyHighSeverityFinding(t *testing.T) {
	findings := []drafts.Finding{{Severity: drafts.SeverityHigh}}
	if !deriveRequireHuman(drafts.VerdictEscalate, findings, "high") {
		t.Error("expected requireHuman = true when a finding is high severity")
	}
}

func TestDeriveRequireHuman_AC13_FalseOnlyForCleanSendAtNonLowConfidence(t *testing.T) {
	if deriveRequireHuman(drafts.VerdictSend, nil, "medium") {
		t.Error("expected requireHuman = false for a clean send at medium confidence")
	}
	if deriveRequireHuman(drafts.VerdictSend, nil, "high") {
		t.Error("expected requireHuman = false for a clean send at high confidence")
	}
}
