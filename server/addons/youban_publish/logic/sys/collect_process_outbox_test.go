package sys

import (
	"testing"
	"time"
)

func TestCollectProcessOutboxLeaseCoversWorkerTimeout(t *testing.T) {
	if collectProcessOutboxLease <= 2*time.Minute {
		t.Fatalf("outbox lease %s must exceed collect worker timeout", collectProcessOutboxLease)
	}
}
