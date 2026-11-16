package cache

import (
	"context"
	"time"

	"hotgo/utility/ownedlease"
)

// NewQueueLease binds a queue ownership lease to the configured Redis adapter.
// It preserves ownedlease token fencing and never falls back to another store.
func NewQueueLease(key, owner string, ttl time.Duration) (*ownedlease.Lease, error) {
	client, err := RedisClient()
	if err != nil {
		return nil, err
	}
	return ownedlease.New(key, owner, ttl, client), nil
}

// AsynqQueueHasWork checks runnable/in-flight activity in one atomic snapshot.
// Completed and archived tasks do not keep idle workers alive. All keys share
// Asynq's existing queue hash tag; this does not change the queue protocol.
func AsynqQueueHasWork(ctx context.Context, queueName string) (bool, error) {
	client, err := RedisClient()
	if err != nil {
		return false, err
	}
	prefix := "asynq:{" + queueName + "}:"
	value, err := client.GroupScript().Eval(ctx, `return redis.call('LLEN',KEYS[1])+redis.call('LLEN',KEYS[2])+redis.call('ZCARD',KEYS[3])+redis.call('ZCARD',KEYS[4])`,
		4, []string{prefix + "pending", prefix + "active", prefix + "scheduled", prefix + "retry"}, nil)
	if err != nil {
		return false, err
	}
	return value.Int64() > 0, nil
}
