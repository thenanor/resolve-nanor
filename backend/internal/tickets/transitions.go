package tickets

// AllowedTransitions mirrors ALLOWED_TRANSITIONS from tickets.service.ts.
var AllowedTransitions = map[Status][]Status{
	StatusNew:             {StatusOpen},
	StatusOpen:            {StatusInProgress},
	StatusInProgress:      {StatusWaitingCustomer, StatusResolved},
	StatusWaitingCustomer: {StatusInProgress},
	StatusResolved:        {StatusClosed, StatusInProgress},
	StatusClosed:          {},
}
