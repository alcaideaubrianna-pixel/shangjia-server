package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCollectMaterialLookbackTimeoutYieldsWithoutTerminalFailure(t *testing.T) {
	parent := context.Background()
	lookup, cancel := context.WithDeadline(parent, time.Now().Add(-time.Second))
	defer cancel()
	databaseError := errors.New("pq: canceling statement due to user request")
	if err := collectMaterialLookbackRetryError(parent, lookup, databaseError); !isCollectProcessRetryError(err) {
		t.Fatalf("lookup deadline should yield a retry, got %v", err)
	}
	if err := collectMaterialLookbackRetryError(parent, lookup, nil); err != nil {
		t.Fatalf("completed lookup must remain successful: %v", err)
	}
	if err := collectMaterialLookbackRetryError(parent, parent, databaseError); err != databaseError {
		t.Fatalf("real DB error must not become a timeout retry: %v", err)
	}
	canceled, stop := context.WithCancel(parent)
	stop()
	if err := collectMaterialLookbackRetryError(canceled, lookup, databaseError); err != databaseError {
		t.Fatalf("caller cancellation must propagate unchanged: %v", err)
	}
}
