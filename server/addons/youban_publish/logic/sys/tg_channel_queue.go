package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Channel queue is a logical ready queue. It is deliberately independent of
// the Asynq transport so the migration can be introduced behind one producer.
// The database job remains the source of truth; Redis only stores ready ids.
const telegramChannelQueuePrefix = "youban:tg:channel:queue:"

func telegramChannelQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelQueuePrefix, channelID)
}

// enqueueTelegramChannelJob adds a job id exactly once. Lower scores are read
// first; the lane is encoded ahead of the creation sequence by the caller.
func enqueueTelegramChannelJob(ctx context.Context, channelID, jobID int64, score int64) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	key := telegramChannelQueueKey(channelID)
	member := strconv.FormatInt(jobID, 10)
	var err error
	// Redis is the ready transport, while the DB Outbox remains the source of
	// truth. Retry transient ZADD failures briefly, but never report success on
	// failure: the caller will keep the Outbox pending for the next recovery.
	for attempt := 1; attempt <= 3; attempt++ {
		_, err = g.Redis().Do(ctx, "ZADD", key, score, member)
		if err == nil {
			return nil
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(attempt*20) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	g.Log().Warning(ctx, "频道队列写入失败，保留Outbox待重试", g.Map{
		"event":     "channel_queue_zadd_failed",
		"channelId": channelID,
		"jobId":     jobID,
		"attempts":  3,
		"error":     err.Error(),
	})
	return err
}

// dequeueTelegramChannelHead atomically claims the current queue head. The
// caller must acquire the channel lease before sending and requeue the id when
// the lease is unavailable.
func dequeueTelegramChannelHead(ctx context.Context, channelID int64) (int64, error) {
	if channelID <= 0 {
		return 0, fmt.Errorf("invalid telegram channel id: %d", channelID)
	}
	// Claim only an item whose retry score is due. ZPOPMIN alone would remove
	// future retries immediately and create a hot loop for media_not_ready jobs.
	// The Lua script keeps inspect-and-remove atomic across workers.
	const claimDueScript = `local v=redis.call('ZRANGE',KEYS[1],0,0,'WITHSCORES'); if #v==0 or tonumber(v[2])>tonumber(ARGV[1]) then return {} end; redis.call('ZREM',KEYS[1],v[1]); return {v[1],v[2]}`
	value, err := g.Redis().Do(ctx, "EVAL", claimDueScript, 1, telegramChannelQueueKey(channelID), strconv.FormatInt(time.Now().UnixMilli(), 10))
	if err != nil {
		return 0, err
	}
	items := value.Array()
	if len(items) < 1 {
		return 0, nil
	}
	// Redis adapters may decode ZPOPMIN as either a flat
	// [member, score] array or a nested [[member, score]] array.
	// Accept both shapes so a successfully claimed job is never treated as
	// an empty queue.
	first := items[0]
	if nested, ok := first.([]interface{}); ok && len(nested) > 0 {
		first = nested[0]
	}
	if encoded, ok := first.(string); ok && len(encoded) > 0 && encoded[0] == '[' {
		var nested []interface{}
		if json.Unmarshal([]byte(encoded), &nested) == nil && len(nested) > 0 {
			first = nested[0]
		}
	}
	return gconv.Int64(first), nil
}

func removeTelegramChannelJob(ctx context.Context, channelID, jobID int64) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	_, err := g.Redis().Do(ctx, "ZREM", telegramChannelQueueKey(channelID), strconv.FormatInt(jobID, 10))
	return err
}
