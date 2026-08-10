package cannedresponses

import "context"

// Filter narrows GET /canned-responses to responses tagged with Tag. An
// empty Tag (the default) means no filter.
type Filter struct {
	Tag string
}

// Repository is the persistence boundary for canned responses.
type Repository interface {
	Create(ctx context.Context, cr *CannedResponse) error
	FindAll(ctx context.Context, filter Filter) ([]CannedResponse, error)
	FindByID(ctx context.Context, id string) (*CannedResponse, error)
	Update(ctx context.Context, cr *CannedResponse) error
	Delete(ctx context.Context, id string) error
}
