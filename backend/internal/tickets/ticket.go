package tickets

type Status string

const (
	StatusNew              Status = "new"
	StatusOpen             Status = "open"
	StatusInProgress       Status = "in_progress"
	StatusWaitingCustomer  Status = "waiting_customer"
	StatusResolved         Status = "resolved"
	StatusClosed           Status = "closed"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

var AllPriorities = []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}

func (p Priority) Valid() bool {
	for _, v := range AllPriorities {
		if v == p {
			return true
		}
	}
	return false
}

// Ticket mirrors the NestJS Ticket entity. Dates are ISO-8601 strings, kept
// as strings end-to-end (rather than time.Time) so JSON responses match the
// reference implementation byte-for-byte.
type Ticket struct {
	ID            string    `json:"id"`
	Subject       string    `json:"subject"`
	Description   string    `json:"description"`
	CustomerEmail string    `json:"customerEmail"`
	Priority      Priority  `json:"priority"`
	Status        Status    `json:"status"`
	Comments      []Comment `json:"comments"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
	ResolvedAt    *string   `json:"resolvedAt"`
}

type Comment struct {
	Seq      int64  `json:"-"`
	ID       string `json:"id"`
	Author   string `json:"author"`
	Body     string `json:"body"`
	Internal bool   `json:"internal"`
	At       string `json:"at"`
}
