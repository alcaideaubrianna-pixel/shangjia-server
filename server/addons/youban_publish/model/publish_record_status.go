package model

import "strings"

// PublishRecordStatus is the API and projection status of one publication job.
// It is separate from a profile's current operation and from queue dispatch.
type PublishRecordStatus string

const (
	PublishRecordStatusPending    PublishRecordStatus = "pending"
	PublishRecordStatusSending    PublishRecordStatus = "sending"
	PublishRecordStatusSuccess    PublishRecordStatus = "success"
	PublishRecordStatusRetryWait  PublishRecordStatus = "failed_retry"
	PublishRecordStatusFailed     PublishRecordStatus = "failed"
	PublishRecordStatusSuperseded PublishRecordStatus = "superseded"
	PublishRecordStatusUnknown    PublishRecordStatus = "unknown"
	PublishRecordStatusCancelled  PublishRecordStatus = "cancelled"
	PublishRecordStatusDeleted    PublishRecordStatus = "deleted"
)

func (s PublishRecordStatus) String() string { return string(s) }

func (s PublishRecordStatus) Valid() bool {
	switch s {
	case PublishRecordStatusPending, PublishRecordStatusSending, PublishRecordStatusSuccess,
		PublishRecordStatusRetryWait, PublishRecordStatusFailed, PublishRecordStatusSuperseded,
		PublishRecordStatusUnknown, PublishRecordStatusCancelled, PublishRecordStatusDeleted:
		return true
	default:
		return false
	}
}

// ParsePublishRecordStatus preserves the legacy "sent" request alias.
// Unknown input must not silently turn into a pending publication.
func ParsePublishRecordStatus(value string) (PublishRecordStatus, bool) {
	value = strings.TrimSpace(value)
	if value == "sent" {
		return PublishRecordStatusSuccess, true
	}
	status := PublishRecordStatus(value)
	return status, status.Valid()
}
