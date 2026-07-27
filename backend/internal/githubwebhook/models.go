package githubwebhook

import "time"

type HandleRequest struct {
	DeliveryID string
	EventType  string
	Signature  string
	Body       []byte
}

type HandleResult struct {
	DeliveryID string    `json:"deliveryId"`
	EventType  string    `json:"eventType"`
	Duplicate  bool      `json:"duplicate"`
	Enqueued   bool      `json:"enqueued"`
	SyncJobID  *string   `json:"syncJobId,omitempty"`
	ReceivedAt time.Time `json:"receivedAt"`
	Action     *string   `json:"action,omitempty"`
}

type payloadEnvelope struct {
	Action     string `json:"action"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}
