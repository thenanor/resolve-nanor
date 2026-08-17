package triage

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"resolve/internal/tickets"
)

// toolInputSchema extracts the input_schema properties map for classify_ticket
// out of a built request, for asserting on enum values without depending on
// the SDK's exact JSON marshaling.
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

func enumOf(t *testing.T, props map[string]any, field string) []string {
	t.Helper()
	prop, ok := props[field].(map[string]any)
	if !ok {
		t.Fatalf("properties[%q] is %T, want map[string]any", field, props[field])
	}
	rawEnum, ok := prop["enum"].([]string)
	if !ok {
		t.Fatalf("properties[%q].enum is %T, want []string", field, prop["enum"])
	}
	return rawEnum
}

func TestBuildRequest_UsesHaikuModel(t *testing.T) {
	req := buildRequest("Can't log in", "Password reset email never arrives")
	if req.Model != anthropic.ModelClaudeHaiku4_5 {
		t.Errorf("Model = %q, want %q", req.Model, anthropic.ModelClaudeHaiku4_5)
	}
}

func TestBuildRequest_ForcesTheClassifyTool(t *testing.T) {
	req := buildRequest("subject", "description")
	name := req.ToolChoice.GetName()
	if name == nil || *name != toolName {
		t.Errorf("ToolChoice name = %v, want %q", name, toolName)
	}
}

func TestBuildRequest_ToolSchemaEnumsMatchTicketsPackage(t *testing.T) {
	req := buildRequest("subject", "description")
	props := toolInputSchema(t, req)

	assertSameSet(t, "category", enumOf(t, props, "category"), enumStrings(tickets.AllCategories))
	assertSameSet(t, "priority", enumOf(t, props, "priority"), enumStrings(tickets.AllPriorities))
}

func TestBuildRequest_ConfidenceSchemaIsZeroToOneNumber(t *testing.T) {
	req := buildRequest("subject", "description")
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

func TestBuildRequest_SchemaRequiresInjectionSuspectedAndReasoning(t *testing.T) {
	req := buildRequest("subject", "description")
	if len(req.Tools) != 1 || req.Tools[0].OfTool == nil {
		t.Fatalf("expected exactly one tool definition, got %d", len(req.Tools))
	}
	required := req.Tools[0].OfTool.InputSchema.Required
	for _, field := range []string{"injectionSuspected", "reasoning"} {
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
		//nolint:staticcheck // strings.Title is deprecated but fine for a plain-ASCII test error message
		if !wantSet[g] {
			t.Errorf("%s enum contains %q, not present in tickets.All%s (drift between triage's tool schema and the tickets package)", field, g, strings.Title(field))
		}
	}
}

func TestBuildRequest_SystemPromptMentionsAllEnumValues(t *testing.T) {
	req := buildRequest("subject", "description")
	if len(req.System) != 1 {
		t.Fatalf("expected exactly one system block, got %d", len(req.System))
	}
	prompt := req.System[0].Text
	for _, c := range tickets.AllCategories {
		if !strings.Contains(prompt, string(c)) {
			t.Errorf("system prompt missing category %q", c)
		}
	}
	for _, p := range tickets.AllPriorities {
		if !strings.Contains(prompt, string(p)) {
			t.Errorf("system prompt missing priority %q", p)
		}
	}
	if !strings.Contains(prompt, "between 0 and 1") {
		t.Errorf("system prompt missing the 0-1 confidence scale description")
	}
}

// toolUseMessage builds a *anthropic.Message carrying a single classify_ticket
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
	msg := toolUseMessage(t, `{"category":"billing","priority":"urgent","confidence":0.9,"injectionSuspected":true,"reasoning":"Mentions a duplicate charge on the invoice."}`)
	result, err := parseResult(msg)
	if err != nil {
		t.Fatalf("parseResult returned error: %v", err)
	}
	if result.Category != "billing" || result.Priority != "urgent" || result.Confidence != 0.9 {
		t.Errorf("result = %+v, want {billing urgent 0.9 ...}", result)
	}
	if !result.InjectionSuspected {
		t.Errorf("result.InjectionSuspected = false, want true")
	}
	if result.Reasoning != "Mentions a duplicate charge on the invoice." {
		t.Errorf("result.Reasoning = %q, want the reasoning from the tool input", result.Reasoning)
	}
}

func TestParseResult_InjectionSuspectedDefaultsFalseWhenOmitted(t *testing.T) {
	msg := toolUseMessage(t, `{"category":"billing","priority":"urgent","confidence":0.9,"reasoning":"Duplicate charge."}`)
	result, err := parseResult(msg)
	if err != nil {
		t.Fatalf("parseResult returned error: %v", err)
	}
	if result.InjectionSuspected {
		t.Errorf("result.InjectionSuspected = true, want false when omitted from tool input")
	}
}

func TestParseResult_NoToolUseBlockIsError(t *testing.T) {
	msg := &anthropic.Message{Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "I decline to classify this."}}}
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error when the response has no tool_use block")
	}
}

func TestParseResult_MalformedJSONIsError(t *testing.T) {
	msg := toolUseMessage(t, `{not valid json`)
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error for malformed tool input JSON")
	}
}

func TestParseResult_OutOfEnumCategoryIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"category":"not_a_category","priority":"urgent","confidence":0.9}`)
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error for an out-of-enum category")
	}
}

func TestParseResult_OutOfEnumPriorityIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"category":"billing","priority":"not_a_priority","confidence":0.9}`)
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error for an out-of-enum priority")
	}
}

func TestParseResult_OutOfRangeConfidenceIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"category":"billing","priority":"urgent","confidence":1.5}`)
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error for a confidence above 1")
	}
}

func TestParseResult_NegativeConfidenceIsError(t *testing.T) {
	msg := toolUseMessage(t, `{"category":"billing","priority":"urgent","confidence":-0.1}`)
	if _, err := parseResult(msg); err == nil {
		t.Fatal("expected an error for a negative confidence")
	}
}
