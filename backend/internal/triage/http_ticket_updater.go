package triage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPTicketUpdater satisfies TicketUpdater by PATCHing the classification
// result back to the main app. X-Actor: triage-service is what threads the
// write through the main app's existing httpx.Actor(r) -> audit-trail path
// unmodified.
type HTTPTicketUpdater struct {
	BaseURL string
	Client  *http.Client
}

func (u *HTTPTicketUpdater) UpdateTriage(ctx context.Context, ticketID, category, priority, confidence string) error {
	body, err := json.Marshal(map[string]string{
		"category":   category,
		"priority":   priority,
		"confidence": confidence,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.BaseURL+"/tickets/"+ticketID+"/triage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor", "triage-service")

	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("main app returned status %d for ticket %s", resp.StatusCode, ticketID)
	}
	return nil
}
