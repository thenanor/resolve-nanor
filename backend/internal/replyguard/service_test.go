package replyguard

import (
	"context"
	"errors"
	"testing"
)

type fakeClassifier struct {
	result Result
	err    error
}

func (f fakeClassifier) Guard(context.Context, GuardInput) (Result, error) {
	return f.result, f.err
}

func TestService_Guard_HappyPath_CleanReplySends(t *testing.T) {
	classifier := fakeClassifier{result: Result{Confidence: 0.9, Reasoning: "clean"}}
	svc := NewService(classifier)

	got, err := svc.Guard(context.Background(), GuardInput{})
	if err != nil {
		t.Fatalf("Guard returned error: %v", err)
	}
	if got.Verdict != VerdictSend {
		t.Errorf("Verdict = %q, want %q", got.Verdict, VerdictSend)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want high", got.Confidence)
	}
	if got.RequireHuman {
		t.Errorf("RequireHuman = true, want false for a clean, high-confidence send")
	}
	if got.Reasoning != "clean" {
		t.Errorf("Reasoning = %q, want %q", got.Reasoning, "clean")
	}
}

func TestService_Guard_ClassifierErrorIsSurfaced(t *testing.T) {
	classifier := fakeClassifier{err: errors.New("anthropic unavailable")}
	svc := NewService(classifier)

	if _, err := svc.Guard(context.Background(), GuardInput{}); err == nil {
		t.Fatal("expected an error when the classifier fails")
	}
}

// --- AC-13/14/15/16: verdict and requireHuman derivation ---

func TestDeriveVerdict_AC14_EmptyFindingsSends(t *testing.T) {
	if got := deriveVerdict(nil, false); got != VerdictSend {
		t.Errorf("deriveVerdict(nil, false) = %q, want send", got)
	}
}

func TestDeriveVerdict_AC14_OnlyLowSeverityFindingsSends(t *testing.T) {
	findings := []Finding{{Severity: SeverityLow}, {Severity: SeverityLow}}
	if got := deriveVerdict(findings, false); got != VerdictSend {
		t.Errorf("deriveVerdict(low-only, false) = %q, want send", got)
	}
}

func TestDeriveVerdict_AC15_AnyHighSeverityFindingEscalates(t *testing.T) {
	findings := []Finding{{Severity: SeverityLow}, {Severity: SeverityHigh}}
	if got := deriveVerdict(findings, false); got != VerdictEscalate {
		t.Errorf("deriveVerdict(with high, false) = %q, want escalate", got)
	}
}

func TestDeriveVerdict_AC15_InjectionSuspectedEscalatesEvenWithoutFindings(t *testing.T) {
	if got := deriveVerdict(nil, true); got != VerdictEscalate {
		t.Errorf("deriveVerdict(nil, true) = %q, want escalate", got)
	}
}

func TestDeriveVerdict_AC16_MediumSeverityWithNoHighOrInjectionRevises(t *testing.T) {
	findings := []Finding{{Severity: SeverityLow}, {Severity: SeverityMedium}}
	if got := deriveVerdict(findings, false); got != VerdictRevise {
		t.Errorf("deriveVerdict(medium, false) = %q, want revise", got)
	}
}

func TestDeriveRequireHuman_AC16_TrueOnEscalate(t *testing.T) {
	if !deriveRequireHuman(VerdictEscalate, nil, "high") {
		t.Error("expected requireHuman = true for an escalate verdict")
	}
}

func TestDeriveRequireHuman_AC16_TrueOnLowConfidenceEvenForSend(t *testing.T) {
	if !deriveRequireHuman(VerdictSend, nil, "low") {
		t.Error("expected requireHuman = true for a send verdict at low confidence")
	}
}

func TestDeriveRequireHuman_AC16_TrueOnAnyHighSeverityFinding(t *testing.T) {
	findings := []Finding{{Severity: SeverityHigh}}
	if !deriveRequireHuman(VerdictEscalate, findings, "high") {
		t.Error("expected requireHuman = true when a finding is high severity")
	}
}

func TestDeriveRequireHuman_AC16_FalseOnlyForCleanSendAtNonLowConfidence(t *testing.T) {
	if deriveRequireHuman(VerdictSend, nil, "medium") {
		t.Error("expected requireHuman = false for a clean send at medium confidence")
	}
	if deriveRequireHuman(VerdictSend, nil, "high") {
		t.Error("expected requireHuman = false for a clean send at high confidence")
	}
}
