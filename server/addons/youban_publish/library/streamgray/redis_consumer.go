package streamgray

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

// StreamEntry 是 Redis Stream 消费适配层返回的最小消息结构。
type StreamEntry struct {
	ID      string
	Payload []byte
}

// RedisGroupClient 是 Consumer Group 所需的 Redis 最小接口。
type RedisGroupClient interface {
	ReadGroup(ctx context.Context, stream, group, consumer, startID string, count int, block time.Duration) ([]StreamEntry, error)
	Ack(ctx context.Context, stream, group, entryID string) error
}

// RedisGroupConsumer 实现解码、处理、成功 ACK 的固定流程。
type RedisGroupConsumer struct {
	client   RedisGroupClient
	group    string
	consumer string
	count    int
	block    time.Duration
}

// NewRedisGroupConsumer 创建 Redis Stream Consumer Group 消费器。
func NewRedisGroupConsumer(client RedisGroupClient, group, consumer string) (*RedisGroupConsumer, error) {
	if client == nil || group == "" || consumer == "" {
		return nil, gerror.New("Redis Stream consumer 参数无效")
	}
	return &RedisGroupConsumer{client: client, group: group, consumer: consumer, count: 20, block: 5 * time.Second}, nil
}

// ConsumeOnce 消费一批消息；handler 成功后才 ACK，失败消息保留 pending。
func (c *RedisGroupConsumer) ConsumeOnce(ctx context.Context, stream, startID string, handler func(context.Context, ReadyEvent) error) (int, error) {
	if !validReadyStream(stream) || handler == nil {
		return 0, gerror.New("Redis Stream 消费参数无效")
	}
	entries, err := c.client.ReadGroup(ctx, stream, c.group, c.consumer, startID, c.count, c.block)
	if err != nil {
		return 0, gerror.Wrap(err, "读取 Redis Stream 消息失败")
	}
	processed := 0
	for _, entry := range entries {
		event, decodeErr := DecodeReadyEvent(entry.Payload)
		if decodeErr != nil {
			return processed, decodeErr
		}
		if err = handler(ctx, event); err != nil {
			return processed, err
		}
		if err = c.client.Ack(ctx, stream, c.group, entry.ID); err != nil {
			return processed, gerror.Wrap(err, "确认 Redis Stream 消息失败")
		}
		processed++
	}
	return processed, nil
}
