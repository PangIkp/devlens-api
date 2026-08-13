package githubwebhook

import "errors"

var (
	ErrInvalidSignature = errors.New("invalid github webhook signature")
	ErrInvalidPayload   = errors.New("invalid github webhook payload")
	ErrMissingDelivery  = errors.New("missing github delivery id")
	ErrMissingEvent     = errors.New("missing github event type")
	ErrDeliveryNotFound = errors.New("github webhook delivery not found")
	ErrRetryNotAllowed  = errors.New("github webhook delivery retry not allowed")
)
