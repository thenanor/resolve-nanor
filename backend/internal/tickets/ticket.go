package tickets

type Status string

const (
	StatusNew             Status = "new"
	StatusOpen            Status = "open"
	StatusInProgress      Status = "in_progress"
	StatusWaitingCustomer Status = "waiting_customer"
	StatusResolved        Status = "resolved"
	StatusClosed          Status = "closed"
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

// Category is assigned by the triage service after a ticket is created; it
// has no equivalent in the NestJS reference.
type Category string

const (
	CategoryBilling        Category = "billing"
	CategoryBug            Category = "bug"
	CategoryAccountAccess  Category = "account_access"
	CategoryFeatureRequest Category = "feature_request"
	CategoryHowTo          Category = "how_to"
	CategoryOther          Category = "other"
)

var AllCategories = []Category{CategoryBilling, CategoryBug, CategoryAccountAccess, CategoryFeatureRequest, CategoryHowTo, CategoryOther}

func (c Category) Valid() bool {
	for _, v := range AllCategories {
		if v == c {
			return true
		}
	}
	return false
}

// Confidence describes how sure the triage service is about a classification.
// It is not persisted on the ticket itself — low-confidence results are
// stored as a pending suggestion instead of being applied directly.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

var AllConfidence = []Confidence{ConfidenceLow, ConfidenceMedium, ConfidenceHigh}

func (c Confidence) Valid() bool {
	for _, v := range AllConfidence {
		if v == c {
			return true
		}
	}
	return false
}

// Ticket mirrors the NestJS Ticket entity. Dates are ISO-8601 strings, kept
// as strings end-to-end (rather than time.Time) so JSON responses match the
// reference implementation byte-for-byte.
type Ticket struct {
	ID              string    `json:"id"`
	Subject         string    `json:"subject"`
	Description     string    `json:"description"`
	CustomerEmail   string    `json:"customerEmail"`
	Priority        Priority  `json:"priority"`
	Category        *Category `json:"category"`
	Status          Status    `json:"status"`
	Comments        []Comment `json:"comments"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
	ResolvedAt      *string   `json:"resolvedAt"`
	PendingCategory *Category `json:"pendingCategory"`
	PendingPriority *Priority `json:"pendingPriority"`
}

type Comment struct {
	Seq      int64  `json:"-"`
	ID       string `json:"id"`
	Author   string `json:"author"`
	Body     string `json:"body"`
	Internal bool   `json:"internal"`
	At       string `json:"at"`
}
