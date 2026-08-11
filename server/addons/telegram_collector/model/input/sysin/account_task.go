package sysin

import (
	"encoding/json"
	"time"
)

const (
	AccountTaskStatusPending     = "pending"
	AccountTaskStatusProcessing  = "processing"
	AccountTaskStatusCompleted   = "completed"
	AccountTaskStatusFailedRetry = "failed_retry"
	AccountTaskStatusDead        = "dead"
	AccountTaskStatusCancelled   = "cancelled"

	AccountTaskTypeHistoryPage   = "history_page"
	AccountTaskTypeMediaDownload = "media_download"
)

type AccountTaskSubmit struct {
	TenantID    int64           `json:"tenantId"`
	AccountID   int64           `json:"accountId"`
	TaskType    string          `json:"taskType"`
	TaskKey     string          `json:"taskKey"`
	Priority    int             `json:"priority"`
	Payload     json.RawMessage `json:"payload"`
	MaxAttempts int             `json:"maxAttempts"`
	NextRunAt   *time.Time      `json:"nextRunAt,omitempty"`
}

type AccountTask struct {
	ID           int64           `json:"id"`
	TenantID     int64           `json:"tenantId"`
	AccountID    int64           `json:"accountId"`
	TaskType     string          `json:"taskType"`
	TaskKey      string          `json:"taskKey"`
	Priority     int             `json:"priority"`
	Status       string          `json:"status"`
	Payload      json.RawMessage `json:"payload"`
	Result       json.RawMessage `json:"result,omitempty"`
	AttemptCount int             `json:"attemptCount"`
	MaxAttempts  int             `json:"maxAttempts"`
	LeaseOwner   string          `json:"leaseOwner"`
	LeaseEpoch   int64           `json:"leaseEpoch"`
	LeaseUntil   *time.Time      `json:"leaseUntil,omitempty"`
	NextRunAt    *time.Time      `json:"nextRunAt,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

type AccountMediaDownloadPayload struct {
	TenantID  int64              `json:"tenantId"`
	AccountID int64              `json:"accountId"`
	Media     CollectorMediaItem `json:"media"`
}

type AccountMediaDownloadResult struct {
	AttachmentID int64              `json:"attachmentId,omitempty"`
	FileURL      string             `json:"fileUrl,omitempty"`
	StoragePath  string             `json:"storagePath,omitempty"`
	Media        CollectorMediaItem `json:"media"`
	ErrorCode    string             `json:"errorCode,omitempty"`
	ErrorMessage string             `json:"errorMessage,omitempty"`
}

type AccountTaskFailure struct {
	TaskID     int64         `json:"taskId"`
	Lease      *AccountLease `json:"lease"`
	Cause      error         `json:"-"`
	RetryDelay time.Duration `json:"retryDelay"`
}
