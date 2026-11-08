package sys

import (
	"encoding/json"

	"github.com/gogf/gf/v2/errors/gerror"
)

const (
	// pushEngineUnifiedStream 表示仅在灰度分支启用 Redis Stream 就绪通知。
	pushEngineUnifiedStream = "unified_stream"
	// redisStreamReadyRealtime 是实时任务就绪事件流。
	redisStreamReadyRealtime = "youban_publish:tg:ready:realtime"
	// redisStreamReadyBulk 是批量和循环任务就绪事件流。
	redisStreamReadyBulk = "youban_publish:tg:ready:bulk"
)

// redisStreamReadyEvent 是数据库任务提交后的轻量就绪通知。
// Stream 只传递 job_id，不承载最终状态，也不直接执行 Telegram 发送。
type redisStreamReadyEvent struct {
	JobID     int64  `json:"job_id"`
	ChannelID int64  `json:"channel_id"`
	Priority  int    `json:"priority"`
	QueueName string `json:"queue_name"`
}

// encodeRedisStreamReadyEvent 将就绪事件编码为 Stream 消息体。
func encodeRedisStreamReadyEvent(event redisStreamReadyEvent) ([]byte, error) {
	if event.JobID <= 0 || event.ChannelID <= 0 {
		return nil, gerror.New("Redis Stream 就绪事件参数无效")
	}
	return json.Marshal(event)
}

// redisStreamPushEngineEnabled 判断账户是否启用 Redis Stream 灰度路径。
func redisStreamPushEngineEnabled(engine string) bool {
	return engine == pushEngineUnifiedStream
}
