package streamgray

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/internal/library/cache"
)

// DecodeXReadGroupResponse 解析 Redis XREADGROUP 的标准嵌套数组响应。
// 只读取 payload 字段，忽略未知字段，便于后续扩展事件元数据。
func DecodeXReadGroupResponse(value *gvar.Var) ([]StreamEntry, error) {
	if value == nil || value.IsNil() {
		return nil, nil
	}
	streams := value.Array()
	entries := make([]StreamEntry, 0)
	for _, streamValue := range streams {
		streamParts := gvar.New(streamValue).Array()
		if len(streamParts) < 2 {
			return nil, gerror.New("Redis XREADGROUP stream 响应格式无效")
		}
		for _, entryValue := range gvar.New(streamParts[1]).Array() {
			entryParts := gvar.New(entryValue).Array()
			if len(entryParts) < 2 {
				return nil, gerror.New("Redis XREADGROUP entry 响应格式无效")
			}
			fields := gvar.New(entryParts[1]).Array()
			entry := StreamEntry{ID: gvar.New(entryParts[0]).String()}
			for i := 0; i+1 < len(fields); i += 2 {
				if gvar.New(fields[i]).String() == "payload" {
					entry.Payload = []byte(gvar.New(fields[i+1]).String())
				}
			}
			if entry.ID == "" || len(entry.Payload) == 0 {
				return nil, gerror.New("Redis XREADGROUP entry 缺少 payload")
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

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

// NewRedisCommandGroupClientDefault 使用标准 Redis Stream 响应解析器创建客户端。
func NewRedisCommandGroupClientDefault(client RedisCommandClient) (*RedisCommandGroupClient, error) {
	return NewRedisCommandGroupClient(client, DecodeXReadGroupResponse)
}

// NewRedisCommandGroupClientFromCache 从 HotGo cache 基础设施创建标准 Consumer Group 客户端。
func NewRedisCommandGroupClientFromCache() (*RedisCommandGroupClient, error) {
	client, err := cache.RedisClient()
	if err != nil {
		return nil, err
	}
	return NewRedisCommandGroupClientDefault(client)
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
