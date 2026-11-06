package streamgray

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
)

// StreamReadDecoder 将 Redis XREADGROUP 原始响应转换为统一消息结构。
type StreamReadDecoder func(*gvar.Var) ([]StreamEntry, error)

// RedisCommandGroupClient 使用项目 Redis 命令接口实现 Consumer Group。
type RedisCommandGroupClient struct {
	client RedisCommandClient
	decode StreamReadDecoder
}

// NewRedisCommandGroupClient 创建命令型 Consumer Group 客户端。
func NewRedisCommandGroupClient(client RedisCommandClient, decode StreamReadDecoder) (*RedisCommandGroupClient, error) {
	if client == nil || decode == nil {
		return nil, gerror.New("Redis Stream group command client 参数无效")
	}
	return &RedisCommandGroupClient{client: client, decode: decode}, nil
}

// ReadGroup 执行 XREADGROUP GROUP 命令，响应解析委托给注入的 decoder。
func (c *RedisCommandGroupClient) ReadGroup(ctx context.Context, stream, group, consumer, startID string, count int, block time.Duration) ([]StreamEntry, error) {
	if stream == "" || group == "" || consumer == "" || startID == "" || count <= 0 {
		return nil, gerror.New("Redis Stream XREADGROUP 参数无效")
	}
	result, err := c.client.Do(ctx, "XREADGROUP", "GROUP", group, consumer, "COUNT", count, "BLOCK", block.Milliseconds(), "STREAMS", stream, startID)
	if err != nil {
		return nil, gerror.Wrap(err, "执行 Redis Stream XREADGROUP 失败")
	}
	return c.decode(result)
}

// Ack 执行 XACK 确认消息。
func (c *RedisCommandGroupClient) Ack(ctx context.Context, stream, group, entryID string) error {
	if stream == "" || group == "" || entryID == "" {
		return gerror.New("Redis Stream XACK 参数无效")
	}
	if _, err := c.client.Do(ctx, "XACK", stream, group, entryID); err != nil {
		return gerror.Wrap(err, "执行 Redis Stream XACK 失败")
	}
	return nil
}
