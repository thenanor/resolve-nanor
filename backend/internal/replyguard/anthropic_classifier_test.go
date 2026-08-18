package replyguard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

)

// toolInputSchema extracts the input_schema properties map for guard_reply
// out of a built request, mirroring triage's anthropic_classifier_test.go.
func toolInputSchema(t *testing.T, req anthropic.MessageNewParams) map[string]any {
	t.Helper()
	if len(req.Tools) != 1 || req.Tools[0].OfTool == nil {
		t.Fatalf("expected exactly one tool definition, got %d", len(req.Tools))
	}
	props, ok := req.Tools[0].OfTool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatalf("input_schema.Properties is %T, want map[string]any", req.Tools[0].OfTool.InputSchema.Properties)
	}
	return props
}

func findingItemEnum(t *testing.T, props map[string]any, field string) []string {
	t.Helper()
	findings, ok := props["findings"].(map[string]any)
	if !ok {
		t.Fatalf("properties[%q] is %T, want map[string]any", "findings", props["findings"])
	}
	items, ok := findings["items"].(map[string]any)
	if !ok {
		t.Fatalf("findings.items is %T, want map[string]any", findings["items"])
	}
	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("findings.items.properties is %T, want map[string]any", items["properties"])
	}
	prop, ok := itemProps[field].(map[string]any)
	if !ok {
		t.Fatalf("findings.items.properties[%q] is %T, want map[string]any", field, itemProps[field])
	}
	rawEnum, ok := prop["enum"].([]string)
	if !ok {
		t.Fatalf("findings.items.properties[%q].enum is %T, want []string", field, prop["enum"])
	}
	return rawEnum
}

func TestBuildRequest_UsesHaikuModel(t *testing.T) {
	req := buildRequest(GuardInput{})
	if req.Model != anthropic.ModelClaudeHaiku4_5 {
		t.Errorf("Model = %q, want %q", req.Model, anthropic.ModelClaudeHaiku4_5)
	}
}

func TestBuildRequest_ForcesTheGuardTool(t *testing.T) {
	req := buildRequest(GuardInput{})
	name := req.ToolChoice.GetName()
	if name == nil || *name != toolName {
		t.Errorf("ToolChoice name = %v, want %q", name, toolName)
	}
}

func TestBuildRequest_FindingSchemaEnumsMatchDraftsPackage(t *testing.T) {
	req := buildRequest(GuardInput{})
	props := toolInputSchema(t, req)

	assertSameSet(t, "policy", findingItemEnum(t, props, "policy"), enumStrings(AllPolicies))
	assertSameSet(t, "severity", findingItemEnum(t, props, "severity"), enumStrings(AllSeverities))
}

func TestBuildRequest_SchemaHasNoVerdictField(t *testing.T) {
	// Verdict is always Go-derived from findings/injectionSuspected (see
	// deriveVerdict) — the model is never asked to self-report it, so
	// there's no chance for "model said X but findings say Y" drift.
	req := buildRequest(GuardInput{})
	props := toolInputSchema(t, req)
	if _, ok := props["verdict"]; ok {
		t.Error("tool schema must not include a verdict field")
	}
}

func TestBuildRequest_ConfidenceSchemaIsZeroToOneNumber(t *testing.T) {
	req := buildRequest(GuardInput{})
	props := toolInputSchema(t, req)

	confidence, ok := props["confidence"].(map[string]any)
	if !ok {
		t.Fatalf("properties[%q] is %T, want map[string]any", "confidence", props["confidence"])
	}
	if confidence["type"] != "number" {
		t.Errorf("confidence.type = %v, want %q", confidence["type"], "number")
	}
	if confidence["minimum"] != 0 {
		t.Errorf("confidence.minimum = %v, want 0", confidence["minimum"])
	}
	if confidence["maximum"] != 1 {
		t.Errorf("confidence.maximum = %v, want 1", confidence["maximum"])
	}
}

func TestBuildRequest_SchemaRequiresFindingsInjectionSuspectedAndReasoning(t *testing.T) {
	req := buildRequest(GuardInput{})
	if len(req.Tools) != 1 || req.Tools[0].OfTool == nil {
		t.Fatalf("expected exactly one tool definition, got %d", len(req.Tools))
	}
	required := req.Tools[0].OfTool.InputSchema.Required
	for _, field := range []string{"findings", "injectionSuspected", "reasoning"} {
		found := false
		for _, r := range required {
			if r == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required = %v, want it to include %q", required, field)
		}
	}
}

func assertSameSet(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s enum = %v, want %v", field, got, want)
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	for _, g := range got {
		if !wantSet[g] {
			t.Errorf("%s enum contains %q, not present in the expected enum (drift in replyguard's tool schema)", field, g)
		}
	}
}

func TestBuildRequest_SystemPromptMentionsAllPolicyLinesAndExcludesGrammar(t *testing.T) {
	req := buildRequest(GuardInput{})
	if len(req.System) != 1 {
		t.Fatalf("expected exactly one system block, got %d", len(req.System))
	}
	prompt := req.System[0].Text
	for _, p := range AllPolicies {
		if !strings.Contains(prompt, string(p)) {
			t.Errorf("system prompt missing policy %q", p)
		}
	}
	if !strings.Contains(strings.ToLower(prompt), "grammar") {
		t.Error("system prompt missing the grammar/word-choice exclusion rule (AC-12)")
	}
	if !strings.Contains(prompt, "disclosure") || !strings.Contains(prompt, "matters most") {
		t.Error("system prompt missing the 'disclosure matters most' emphasis")
	}
}

// toolUseMessage builds a *anthropic.Message carrying a single guard_reply
// tool_use block with the given raw JSON input, for parseResult tests.
func toolUseMessage(t *testing.T, input string) *anthropic.Message {
	t.Helper()
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "tool_use", Name: toolName, Input: json.RawMessage(input)},
		},
	}
}

func TestParseResult_ValidToolUse(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[{"policy":"disclosure","severity":"high","issue":"leaks internal note","quote":"already refunded"}],"confidence":0.9,"injectionSuspected":true,"reasoning":"Reveals internal refund status."}`)
	result, err := parseResult(msg, "We noticed the account was already refunded internally.")
	if err != nil {
		t.Fatalf("parseResult returned error: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Policy != PolicyDisclosure || result.Findings[0].Severity != SeverityHigh {
		t.Errorf("Findings = %+v", result.Findings)
	}
	if result.Findings[0].Quote != "already refunded" {
		t.Errorf("Quote = %q, want %q", result.Findings[0].Quote, "already refunded")
	}
	if result.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", result.Confidence)
	}
	if !result.InjectionSuspected {
		t.Error("InjectionSuspected = false, want true")
	}
}

func TestParseResult_EmptyFindingsIsValid(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[],"confidence":0.9,"injectionSuspected":false,"reasoning":"clean"}`)
	result, err := parseResult(msg, "any draft body")
	if err != nil {
		t.Fatalf("parseResult returned error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("Findings = %+v, want empty", result.Findings)
	}
}

func TestParseResult_AC9_QuoteNotFoundVerbatimInDraftIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[{"policy":"tone","severity":"low","issue":"a bit blunt","quote":"this exact text is not in the draft"}],"confidence":0.8,"injectionSuspected":false,"reasoning":"r"}`)
	if _, err := parseResult(msg, "Thanks for reaching out, here is an update."); err == nil {
		t.Fatal("expected an error when a finding's quote is not a literal substring of the draft body")
	}
}

func TestParseResult_InvalidPolicyIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[{"policy":"not_a_policy","severity":"low","issue":"i","quote":"draft"}],"confidence":0.8,"injectionSuspected":false,"reasoning":"r"}`)
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error for an out-of-enum policy")
	}
}

func TestParseResult_InvalidSeverityIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[{"policy":"tone","severity":"not_a_severity","issue":"i","quote":"draft"}],"confidence":0.8,"injectionSuspected":false,"reasoning":"r"}`)
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error for an out-of-enum severity")
	}
}

func TestParseResult_NoToolUseBlockIsError(t *testing.T) {
	msg := &anthropic.Message{Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "I decline to assess this."}}}
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error when the response has no tool_use block")
	}
}

func TestParseResult_MalformedJSONIsError(t *testing.T) {
	msg := toolUseMessage(t, `{not valid json`)
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error for malformed tool input JSON")
	}
}

func TestParseResult_OutOfRangeConfidenceIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[],"confidence":1.5,"injectionSuspected":false,"reasoning":"r"}`)
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error for a confidence above 1")
	}
}

func TestParseResult_NegativeConfidenceIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"findings":[],"confidence":-0.1,"injectionSuspected":false,"reasoning":"r"}`)
	if _, err := parseResult(msg, "draft"); err == nil {
		t.Fatal("expected an error for a negative confidence")
	}
}
