package githubwebhook

import "time"

type HandleRequest struct {
	DeliveryID string
	EventType  string
	Signature  string
	Body       []byte
}

type StoredDelivery struct {
	DeliveryID       string
	EventType        string
	Action           *string
	RepositoryID     *string
	InstallationID   *int64
	Payload          []byte
	ProcessingStatus string
	SyncJobID        *string
	ReceivedAt       time.Time
	RetryCount       int
}

type HandleResult struct {
	DeliveryID       string    `json:"deliveryId"`
	EventType        string    `json:"eventType"`
	Duplicate        bool      `json:"duplicate"`
	Enqueued         bool      `json:"enqueued"`
	ProcessingStatus string    `json:"processingStatus"`
	SyncJobID        *string   `json:"syncJobId,omitempty"`
	ReceivedAt       time.Time `json:"receivedAt"`
	Action           *string   `json:"action,omitempty"`
}

type payloadEnvelope struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}
