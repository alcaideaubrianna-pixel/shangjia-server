package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Channel queue is a logical ready queue. It is deliberately independent of
// the Asynq transport so the migration can be introduced behind one producer.
// The database job remains the source of truth; Redis only stores ready ids.
const telegramChannelQueuePrefix = "youban:tg:channel:queue:"
const telegramChannelUrgentQueuePrefix = "youban:tg:channel:urgent:"
const telegramChannelHistoryQueuePrefix = "youban:tg:channel:history:"
const telegramChannelActiveSetKey = "youban:tg:channel:active"

func telegramChannelQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelQueuePrefix, channelID)
}

func telegramChannelUrgentQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelUrgentQueuePrefix, channelID)
}

func telegramChannelHistoryQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelHistoryQueuePrefix, channelID)
}

func telegramChannelQueueKeyForPriority(channelID int64, priority int) string {
	if priority <= tgJobPriorityInteractive {
		return telegramChannelUrgentQueueKey(channelID)
	}
	if priority >= tgJobPriorityHistory {
		return telegramChannelHistoryQueueKey(channelID)
	}
	return telegramChannelQueueKey(channelID)
}

const (
	// Redis stores scores as IEEE-754 doubles. Keep the complete score below
	// 2^53 while reserving the high part for the lane and the low part for the
	// due timestamp plus a FIFO tie breaker.
	telegramChannelQueuePriorityScale int64 = 1_800_000_000_000_000
	telegramChannelQueueSequenceScale int64 = 1_000
)

// telegramChannelQueuePriorityRank maps the existing publish lanes to the
// queue order. Lower ranks are consumed first; manual/interactive work is
// therefore ahead of realtime, normal, bulk and historical work.
func telegramChannelQueuePriorityRank(priority int) int64 {
	switch {
	case priority <= tgJobPriorityInteractive:
		return 0
	case priority <= tgJobPriorityUrgent:
		return 1
	case priority <= tgJobPriorityDefault:
		return 2
	case priority <= tgJobPriorityBulk:
		return 3
	default:
		return 4
	}
}

// telegramChannelQueueScore implements priority > available_at > FIFO.
// Job IDs are monotonically allocated by PostgreSQL and provide a stable FIFO
// tie breaker for jobs made available in the same millisecond. The queue
// worker decodes only the due timestamp for readiness checks.
func telegramChannelQueueScore(priority int, availableAt time.Time, jobID int64) int64 {
	if availableAt.IsZero() {
		availableAt = time.Now()
	}
	dueMillis := availableAt.UnixMilli()
	if dueMillis < 0 {
		dueMillis = 0
	}
	sequence := jobID % telegramChannelQueueSequenceScale
	if sequence < 0 {
		sequence = -sequence
	}
	return telegramChannelQueuePriorityRank(priority)*telegramChannelQueuePriorityScale + dueMillis*telegramChannelQueueSequenceScale + sequence
}

func telegramChannelQueueNeedsMigration(score int64, priority int, urgent bool, history ...bool) bool {
	expectedHistory := priority >= tgJobPriorityHistory
	actualHistory := len(history) > 0 && history[0]
	if actualHistory != expectedHistory {
		return true
	}
	expectedUrgent := priority <= tgJobPriorityInteractive
	if urgent != expectedUrgent {
		return true
	}
	if expectedUrgent || expectedHistory {
		return false
	}
	return score/telegramChannelQueuePriorityScale != telegramChannelQueuePriorityRank(priority)
}

// enqueueTelegramChannelJob adds a job id exactly once. Lower scores are read
// first; the lane is encoded ahead of the creation sequence by the caller.
func enqueueTelegramChannelJob(ctx context.Context, channelID, jobID int64, priority int, availableAt time.Time) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	key := telegramChannelQueueKeyForPriority(channelID, priority)
	member := strconv.FormatInt(jobID, 10)
	score := telegramChannelQueueScore(priority, availableAt, jobID)
	otherKeys := make([]string, 0, 2)
	for _, candidate := range []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
		if candidate != key {
			otherKeys = append(otherKeys, candidate)
		}
	}
	// Keep the queue migration atomic while limiting the script to one channel
	// and one job. This replaces four Redis round trips with one EVAL and keeps
	// the PostgreSQL Outbox as the durable source of truth.
	const enqueueChannelJobScript = `redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2]); redis.call('ZREM',KEYS[3],ARGV[2]); redis.call('ZREM',KEYS[4],ARGV[2]); redis.call('SADD',KEYS[2],ARGV[3]); return 1`
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		startedAt := time.Now()
		_, err = g.Redis().Do(ctx, "EVAL", enqueueChannelJobScript, 4, key, telegramChannelActiveSetKey, otherKeys[0], otherKeys[1], strconv.FormatInt(score, 10), member, strconv.FormatInt(channelID, 10))
		result := "success"
		if err != nil {
			result = "error"
		}
		observeRedisLua(ctx, "telegram_channel_enqueue", result, time.Since(startedAt))
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
		"event":       "channel_queue_enqueue_failed",
		"channelId":   channelID,
		"jobId":       jobID,
		"priority":    priority,
		"availableAt": availableAt,
		"score":       score,
		"queue":       key,
		"attempts":    3,
		"error":       err.Error(),
	})
	return err
}

func activeTelegramChannelIDs(ctx context.Context) ([]int64, error) {
	value, err := g.Redis().Do(ctx, "SMEMBERS", telegramChannelActiveSetKey)
	if err != nil {
		return nil, err
	}
	var result []int64
	for _, item := range value.Array() {
		id, parseErr := strconv.ParseInt(fmt.Sprint(item), 10, 64)
		if parseErr == nil && id > 0 {
			result = append(result, id)
		}
	}
	return result, nil
}

func removeActiveTelegramChannel(ctx context.Context, channelID int64) error {
	if channelID <= 0 {
		return nil
	}
	_, err := g.Redis().Do(ctx, "SREM", telegramChannelActiveSetKey, strconv.FormatInt(channelID, 10))
	return err
}

type telegramChannelQueueClaim struct {
	JobID   int64
	Score   int64
	Urgent  bool
	History bool
}

func dequeueTelegramChannelHeadFromKey(ctx context.Context, key string) (telegramChannelQueueClaim, error) {
	// Claim only an item whose encoded available_at is due. The priority lane
	// remains part of the score for ordering, but readiness must inspect the
	// due-time portion only; comparing the complete score would make every
	// priority lane appear to be in the far future.
	const claimDueScript = `local v=redis.call('ZRANGE',KEYS[1],0,0,'WITHSCORES'); if #v==0 then return {} end; local score=tonumber(v[2]); local due=math.floor((score % 1800000000000000) / 1000); if due>tonumber(ARGV[1]) then return {} end; redis.call('ZREM',KEYS[1],v[1]); return {v[1],v[2]}`
	startedAt := time.Now()
	value, err := g.Redis().Do(ctx, "EVAL", claimDueScript, 1, key, strconv.FormatInt(time.Now().UnixMilli(), 10))
	result := "success"
	if err != nil {
		result = "error"
	}
	observeRedisLua(ctx, "telegram_channel_claim_due", result, time.Since(startedAt))
	if err != nil {
		return telegramChannelQueueClaim{}, err
	}
	items := value.Array()
	if len(items) < 1 {
		return telegramChannelQueueClaim{}, nil
	}
	first := items[0]
	second := interface{}(nil)
	if nested, ok := first.([]interface{}); ok {
		if len(nested) > 0 {
			first = nested[0]
		}
		if len(nested) > 1 {
			second = nested[1]
		}
	} else if len(items) > 1 {
		second = items[1]
	}
	if encoded, ok := first.(string); ok && len(encoded) > 0 && encoded[0] == '[' {
		var nested []interface{}
		if json.Unmarshal([]byte(encoded), &nested) == nil && len(nested) > 0 {
			first = nested[0]
			if len(nested) > 1 {
				second = nested[1]
			}
		}
	}
	jobID := gconv.Int64(first)
	if jobID <= 0 {
		return telegramChannelQueueClaim{}, nil
	}
	return telegramChannelQueueClaim{
		JobID:   jobID,
		Score:   gconv.Int64(second),
		Urgent:  strings.HasPrefix(key, telegramChannelUrgentQueuePrefix),
		History: strings.HasPrefix(key, telegramChannelHistoryQueuePrefix),
	}, nil
}

// dequeueTelegramChannelHead atomically claims the current queue head. The
// caller must acquire the channel lease before sending and requeue the id when
// the lease is unavailable.
func dequeueTelegramChannelHead(ctx context.Context, channelID int64) (int64, error) {
	claim, err := dequeueTelegramChannelHeadWithScore(ctx, channelID)
	return claim.JobID, err
}

func dequeueTelegramChannelHeadWithScore(ctx context.Context, channelID int64) (telegramChannelQueueClaim, error) {
	if channelID <= 0 {
		return telegramChannelQueueClaim{}, fmt.Errorf("invalid telegram channel id: %d", channelID)
	}
	// A future urgent retry must not block a due ordinary task. Check urgent
	// first, then ordinary, and let each ZSET independently decide readiness.
	for _, key := range []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
		claim, err := dequeueTelegramChannelHeadFromKey(ctx, key)
		if err != nil {
			return telegramChannelQueueClaim{}, err
		}
		if claim.JobID > 0 {
			return claim, nil
		}
	}
	return telegramChannelQueueClaim{}, nil
}

func removeTelegramChannelJob(ctx context.Context, channelID, jobID int64) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	member := strconv.FormatInt(jobID, 10)
	for _, key := range []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
		if _, err := g.Redis().Do(ctx, "ZREM", key, member); err != nil {
			return err
		}
	}
	return nil
}
