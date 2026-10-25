package sys

import (
	"errors"
	"testing"
	"time"
)

func TestCollectProcessOutboxLeaseCoversWorkerTimeout(t *testing.T) {
	if collectProcessOutboxLease <= 2*time.Minute {
		t.Fatalf("outbox lease %s must exceed collect worker timeout", collectProcessOutboxLease)
	}
}

func TestCollectProcessRedisTransientError(t *testing.T) {
	transient := []error{
		errors.New("redis eval error: LOADING Redis is loading the dataset in memory"),
		errors.New("redis client: dial tcp: connection refused"),
		errors.New("cache redis: context deadline exceeded"),
	}
	for _, err := range transient {
		if !isCollectProcessRedisTransientError(err) {
			t.Fatalf("expected transient Redis error: %v", err)
		}
	}
	for _, err := range []error{
		errors.New("资料格式不合法"),
		errors.New("database query context deadline exceeded"),
	} {
		if isCollectProcessRedisTransientError(err) {
			t.Fatalf("non-Redis errors must remain permanent collection failures: %v", err)
		}
	}
}

func TestCollectProcessRedisRetryDelayIsBounded(t *testing.T) {
	if collectProcessRedisRetryDelay < 10*time.Second || collectProcessRedisRetryDelay > time.Minute {
		t.Fatalf("unexpected Redis retry delay: %s", collectProcessRedisRetryDelay)
	}
}
