package sys

import (
	"context"
	"fmt"
	"strconv"

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
	_, err := g.Redis().Do(ctx, "ZADD", telegramChannelQueueKey(channelID), score, strconv.FormatInt(jobID, 10))
	return err
}

// dequeueTelegramChannelHead atomically claims the current queue head. The
// caller must acquire the channel lease before sending and requeue the id when
// the lease is unavailable.
func dequeueTelegramChannelHead(ctx context.Context, channelID int64) (int64, error) {
	if channelID <= 0 {
		return 0, fmt.Errorf("invalid telegram channel id: %d", channelID)
	}
	value, err := g.Redis().Do(ctx, "ZPOPMIN", telegramChannelQueueKey(channelID), 1)
	if err != nil {
		return 0, err
	}
	items := value.Array()
	if len(items) < 1 {
		return 0, nil
	}
	return gconv.Int64(items[0]), nil
}

func removeTelegramChannelJob(ctx context.Context, channelID, jobID int64) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	_, err := g.Redis().Do(ctx, "ZREM", telegramChannelQueueKey(channelID), strconv.FormatInt(jobID, 10))
	return err
}
