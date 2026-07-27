package githubwebhook

import "errors"

var (
	ErrInvalidSignature = errors.New("invalid github webhook signature")
	ErrMissingDelivery  = errors.New("missing github delivery id")
	ErrMissingEvent     = errors.New("missing github event type")
)
