package sys

import (
	"database/sql"
	"errors"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectOptionalRecordScanSucceeded(t *testing.T) {
	if !collectOptionalRecordScanSucceeded(nil) {
		t.Fatal("nil error should be accepted")
	}
	if !collectOptionalRecordScanSucceeded(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should enter the record creation path")
	}
	if collectOptionalRecordScanSucceeded(errors.New("database unavailable")) {
		t.Fatal("unexpected database errors must be returned")
	}
}

func TestBotCollectChannelRelationEnabledAcceptsLegacyApproval(t *testing.T) {
	if !botCollectChannelRelationEnabled(1, "approved", "approved") {
		t.Fatal("legacy approved relation should remain enabled")
	}
	if !botCollectChannelRelationEnabled(1, sysin.CollectAccountChannelEnabled, "pending") {
		t.Fatal("current enabled relation should remain enabled")
	}
	if botCollectChannelRelationEnabled(0, "approved", "approved") {
		t.Fatal("disabled collection must not be enabled by approval status")
	}
}

func TestBotCollectChannelDefaultEnabled(t *testing.T) {
	for _, tc := range []struct {
		status, approval string
		enabled          int
		want             bool
	}{
		{"", "", 0, true}, {"pending", "pending", 0, true},
		{"disabled", "approved", 1, false}, {"enabled", "rejected", 1, false},
		{"enabled", "approved", 1, true}, {"enabled", "approved", 0, false},
	} {
		if got := botCollectChannelRelationEnabled(tc.enabled, tc.status, tc.approval); got != tc.want {
			t.Fatalf("%+v: got %v", tc, got)
		}
	}
}
