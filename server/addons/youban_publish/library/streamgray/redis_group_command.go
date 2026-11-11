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
	// go-redis/GoFrame 在空结果时返回 map[interface{}]interface{}，而非标准嵌套数组。
	if raw, ok := value.Val().(map[interface{}]interface{}); ok {
		if len(raw) == 0 {
			return nil, nil
		}
		for _, streamValue := range raw {
			if entries, ok := streamValue.([]interface{}); ok && len(entries) == 0 {
				return nil, nil
			}
		}
	}
	// Redis adapter may normalize the response to a string-keyed map.
	if raw := value.Map(); len(raw) > 0 {
		for _, streamValue := range raw {
			if entries, ok := streamValue.([]interface{}); ok {
				if len(entries) == 0 {
					return nil, nil
				}
				// Convert map form to the standard [stream, entries] shape.
				streams := []any{[]any{"stream", entries}}
				return decodeXReadStreams(streams)
			}
		}
	}
	streams := value.Array()
	return decodeXReadStreams(streams)
}

func decodeXReadStreams(streams []any) ([]StreamEntry, error) {
	if len(streams) == 0 {
		return nil, nil
	}
	entries := make([]StreamEntry, 0)
	for _, streamValue := range streams {
		if gvar.New(streamValue).IsNil() {
			continue
		}
		streamParts := gvar.New(streamValue).Array()
		if len(streamParts) < 2 {
			return nil, gerror.Newf("Redis XREADGROUP stream 响应格式无效 type=%T value=%v", streamValue, streamValue)
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

// DecodeXAutoClaimResponse 解析 XAUTOCLAIM 返回的 [next-id, entries, deleted]。
func DecodeXAutoClaimResponse(value *gvar.Var) ([]StreamEntry, error) {
	if value == nil || value.IsNil() {
		return nil, nil
	}
	parts := value.Array()
	if len(parts) < 2 {
		return nil, gerror.New("Redis XAUTOCLAIM 响应格式无效")
	}
	return decodeStreamEntries(gvar.New(parts[1]).Array())
}

func decodeStreamEntries(values []any) ([]StreamEntry, error) {
	entries := make([]StreamEntry, 0, len(values))
	for _, entryValue := range values {
		entryParts := gvar.New(entryValue).Array()
		if len(entryParts) < 2 {
			return nil, gerror.New("Redis Stream entry 响应格式无效")
		}
		fields := gvar.New(entryParts[1]).Array()
		entry := StreamEntry{ID: gvar.New(entryParts[0]).String()}
		for i := 0; i+1 < len(fields); i += 2 {
			if gvar.New(fields[i]).String() == "payload" {
				entry.Payload = []byte(gvar.New(fields[i+1]).String())
			}
		}
		if entry.ID == "" || len(entry.Payload) == 0 {
			return nil, gerror.New("Redis Stream entry 缺少 payload")
		}
		entries = append(entries, entry)
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

// CreateGroup 创建 Consumer Group；MKSTREAM 确保首次部署时 Stream 不存在也能启动。
func (c *RedisCommandGroupClient) CreateGroup(ctx context.Context, stream, group string) error {
	if stream == "" || group == "" {
		return gerror.New("Redis Stream XGROUP 参数无效")
	}
	if _, err := c.client.Do(ctx, "XGROUP", "CREATE", stream, group, "$", "MKSTREAM"); err != nil {
		return gerror.Wrap(err, "创建 Redis Stream consumer group 失败")
	}
	return nil
}

// ClaimPending 使用 XAUTOCLAIM 接管其他 consumer 遗留的超时消息。
func (c *RedisCommandGroupClient) ClaimPending(ctx context.Context, stream, group, consumer string, minIdle time.Duration, count int) ([]StreamEntry, error) {
	if stream == "" || group == "" || consumer == "" || count <= 0 {
		return nil, gerror.New("Redis Stream XAUTOCLAIM 参数无效")
	}
	result, err := c.client.Do(ctx, "XAUTOCLAIM", stream, group, consumer, minIdle.Milliseconds(), "0-0", "COUNT", count)
	if err != nil {
		return nil, gerror.Wrap(err, "执行 Redis Stream XAUTOCLAIM 失败")
	}
	return DecodeXAutoClaimResponse(result)
}
