package sysin

import "time"

type AccountLease struct {
	AccountID  int64     `json:"accountId"`
	InstanceID string    `json:"instanceId"`
	Epoch      int64     `json:"epoch"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
