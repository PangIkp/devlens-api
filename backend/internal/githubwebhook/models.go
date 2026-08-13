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
	PullRequest struct {
		ID           int64      `json:"id"`
		Number       int        `json:"number"`
		Title        string     `json:"title"`
		State        string     `json:"state"`
		Draft        bool       `json:"draft"`
		CreatedAt    time.Time  `json:"created_at"`
		UpdatedAt    time.Time  `json:"updated_at"`
		ClosedAt     *time.Time `json:"closed_at"`
		MergedAt     *time.Time `json:"merged_at"`
		Additions    int        `json:"additions"`
		Deletions    int        `json:"deletions"`
		ChangedFiles int        `json:"changed_files"`
		User         struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
	Review struct {
		ID          int64      `json:"id"`
		State       string     `json:"state"`
		SubmittedAt *time.Time `json:"submitted_at"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"review"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Timestamp *time.Time `json:"timestamp"`
		Author  struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
	} `json:"commits"`
	WorkflowRun struct {
		ID           int64      `json:"id"`
		Name         string     `json:"name"`
		Status       string     `json:"status"`
		Conclusion   string     `json:"conclusion"`
		RunStartedAt *time.Time `json:"run_started_at"`
		CreatedAt    time.Time  `json:"created_at"`
		UpdatedAt    time.Time  `json:"updated_at"`
	} `json:"workflow_run"`
	Deployment struct {
		ID          int64     `json:"id"`
		Environment string    `json:"environment"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	} `json:"deployment"`
	DeploymentStatus struct {
		ID        int64     `json:"id"`
		State     string    `json:"state"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"deployment_status"`
}
