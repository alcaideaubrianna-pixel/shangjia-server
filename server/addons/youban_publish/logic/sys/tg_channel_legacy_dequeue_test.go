package sys

// 旧破坏式出队仅供旧 score/lane 回归测试。生产代码只能调用可靠 claim。
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"strconv"
	"strings"
	"time"
)

func dequeueTelegramChannelHeadFromKey(ctx context.Context, key string) (telegramChannelQueueClaim, error) {
	return dequeueTelegramChannelHeadFromKeys(ctx, []string{key})
}

func dequeueTelegramChannelHeadFromKeys(ctx context.Context, keys []string) (telegramChannelQueueClaim, error) {
	// Claim only an item whose encoded available_at is due. The priority lane
	// remains part of the score for ordering, but readiness must inspect the
	// due-time portion only; comparing the complete score would make every
	// priority lane appear to be in the far future.
	// Inspect at most three heads in lane order and remove at most one job.
	const claimDueScript = `for i=1,math.min(#KEYS,3) do local v=redis.call('ZRANGE',KEYS[i],0,0,'WITHSCORES'); if #v>0 then local due=math.floor(tonumber(v[2]) % 2000000000000); if due<=tonumber(ARGV[1]) then redis.call('ZREM',KEYS[i],v[1]); return {v[1],v[2],KEYS[i]} end end end; return {}`
	args := []interface{}{claimDueScript, len(keys)}
	for _, key := range keys {
		args = append(args, key)
	}
	args = append(args, strconv.FormatInt(time.Now().UnixMilli(), 10))
	// This measures client RTT, including pool/network/server waiting, not Lua lock time.
	startedAt := time.Now()
	value, err := g.Redis().Do(ctx, "EVAL", args...)
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
	lane := interface{}(nil)
	if nested, ok := first.([]interface{}); ok {
		if len(nested) > 0 {
			first = nested[0]
		}
		if len(nested) > 1 {
			second = nested[1]
		}
		if len(nested) > 2 {
			lane = nested[2]
		}
	} else if len(items) > 1 {
		second = items[1]
		if len(items) > 2 {
			lane = items[2]
		}
	}
	if encoded, ok := first.(string); ok && len(encoded) > 0 && encoded[0] == '[' {
		var nested []interface{}
		if json.Unmarshal([]byte(encoded), &nested) == nil && len(nested) > 0 {
			first = nested[0]
			if len(nested) > 1 {
				second = nested[1]
			}
			if len(nested) > 2 {
				lane = nested[2]
			}
		}
	}
	jobID := gconv.Int64(first)
	if jobID <= 0 {
		return telegramChannelQueueClaim{}, nil
	}
	key := gconv.String(lane)
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
	// A future head in one lane must not block a due head in the next lane.
	return dequeueTelegramChannelHeadFromKeys(ctx, []string{
		telegramChannelUrgentQueueKey(channelID),
		telegramChannelQueueKey(channelID),
		telegramChannelHistoryQueueKey(channelID),
	})
}
