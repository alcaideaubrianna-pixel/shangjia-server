package streamgray

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

const streamConsumerIdleWait = 100 * time.Millisecond

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

// RedisGroupAdmin 是可选的 Consumer Group 初始化能力。
type RedisGroupAdmin interface {
	CreateGroup(context.Context, string, string) error
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

// Run 持续消费一个 Stream；启动时先读取 pending，再读取新消息。
// Redis 阻塞读取由 client 控制，空批次仅短暂让出调度，避免无消息时忙循环。
func (c *RedisGroupConsumer) Run(ctx context.Context, stream string, handler func(context.Context, ReadyEvent) error) error {
	if !validReadyStream(stream) || handler == nil {
		return gerror.New("Redis Stream 运行参数无效")
	}
	if admin, ok := c.client.(RedisGroupAdmin); ok {
		if err := admin.CreateGroup(ctx, stream, c.group); err != nil && !isGroupAlreadyExistsError(err) {
			return gerror.Wrap(err, "初始化 Redis Stream consumer group 失败")
		}
	}
	startID := "0"
	for {
		processed, err := c.ConsumeOnce(ctx, stream, startID, handler)
		if err != nil {
			return err
		}
		// pending 读完后切换到新消息；后续始终使用 >，避免重复扫描历史消息。
		startID = ">"
		if processed > 0 {
			continue
		}
		timer := time.NewTimer(streamConsumerIdleWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// isGroupAlreadyExistsError 由具体 Redis 适配器将 BUSYGROUP 等错误映射为可忽略状态。
// 默认只识别稳定错误文本，避免吞掉连接或权限错误。
func isGroupAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") || strings.Contains(strings.ToLower(err.Error()), "already exists")
}
