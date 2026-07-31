package sys

import (
	"database/sql"
	"errors"
	"testing"
)

func TestChannelStableIdentityScanSucceeded(t *testing.T) {
	if !channelStableIdentityScanSucceeded(nil) {
		t.Fatal("nil error should be accepted")
	}
	if !channelStableIdentityScanSucceeded(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should be treated as a missing channel")
	}
	if channelStableIdentityScanSucceeded(errors.New("query failed")) {
		t.Fatal("unexpected query errors must be returned")
	}
}
