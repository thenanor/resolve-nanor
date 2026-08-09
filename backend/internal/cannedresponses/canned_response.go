package cannedresponses

// CannedResponse is reusable comment text an agent can insert into a ticket
// comment. Tags are always trimmed, lower-cased, non-empty strings (see
// Service.normalizeTags); the slice itself may be empty but is never nil so
// it serializes as [] rather than null.
type CannedResponse struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}
