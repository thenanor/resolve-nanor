package replyguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"resolve/internal/drafts"
)

// HTTPDraftUpdater satisfies DraftUpdater by POSTing the guard result back
// to the main app. X-Actor: reply-guard-service is what threads the write
// through the main app's existing httpx.Actor(r) -> audit-trail path
// unmodified, mirroring triage's HTTPTicketUpdater.
type HTTPDraftUpdater struct {
	BaseURL string
	Client  *http.Client
}

func (u *HTTPDraftUpdater) UpdateGuardResult(ctx context.Context, ticketID, draftID string, verdict string, findings []drafts.Finding, confidence, reasoning string, injectionSuspected, requireHuman bool) error {
	body, err := json.Marshal(map[string]any{
		"verdict":            verdict,
		"findings":           findings,
		"confidence":         confidence,
		"reasoning":          reasoning,
		"injectionSuspected": injectionSuspected,
		"requireHuman":       requireHuman,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/tickets/%s/drafts/%s/guard-result", u.BaseURL, ticketID, draftID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor", "reply-guard-service")

	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("main app returned status %d for draft %s (ticket %s)", resp.StatusCode, draftID, ticketID)
	}
	return nil
}
