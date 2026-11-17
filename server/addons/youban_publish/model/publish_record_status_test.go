package model

import (
	"encoding/json"
	"testing"
)

func TestParsePublishRecordStatus(t *testing.T) {
	for _, status := range []PublishRecordStatus{
		PublishRecordStatusPending, PublishRecordStatusSending, PublishRecordStatusSuccess,
		PublishRecordStatusRetryWait, PublishRecordStatusFailed, PublishRecordStatusSuperseded,
		PublishRecordStatusUnknown, PublishRecordStatusCancelled, PublishRecordStatusDeleted,
	} {
		got, ok := ParsePublishRecordStatus(" " + status.String() + " ")
		if !ok || got != status || !got.Valid() {
			t.Fatalf("parse %q = %q, %v", status, got, ok)
		}
		data, err := json.Marshal(struct {
			Status PublishRecordStatus `json:"status"`
		}{status})
		if err != nil || string(data) != `{"status":"`+status.String()+`"}` {
			t.Fatalf("status JSON changed: %s %v", data, err)
		}
	}
	if got, ok := ParsePublishRecordStatus("sent"); !ok || got != PublishRecordStatusSuccess {
		t.Fatalf("legacy sent alias = %q, %v", got, ok)
	}
	for _, raw := range []string{"", "unexpected", "processing", "success;DROP TABLE x"} {
		if _, ok := ParsePublishRecordStatus(raw); ok {
			t.Fatalf("accepted invalid record status %q", raw)
		}
	}
}
