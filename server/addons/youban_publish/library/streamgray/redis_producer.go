package streamgray

import (
	"context"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/internal/library/cache"
)

// RedisCommandClient 是 GoFrame Redis 客户端的最小依赖接口。
// 通过接口注入，Stream 包不直接获取全局 Redis 实例。
type RedisCommandClient interface {
	Do(ctx context.Context, command string, args ...any) (*gvar.Var, error)
}

// NewRedisProducerFromCache 从 HotGo cache 基础设施创建 Producer。
// 业务层不直接获取 Redis 客户端；非 Redis 缓存驱动会明确返回错误。
func NewRedisProducerFromCache() (*RedisProducer, error) {
	client, err := cache.RedisClient()
	if err != nil {
		return nil, err
	}
	return NewRedisProducer(client)
}

// RedisProducer 使用 XADD 写入灰度就绪 Stream。
type RedisProducer struct {
	client RedisCommandClient
}

// NewRedisProducer 创建 Redis Stream 生产者。
func NewRedisProducer(client RedisCommandClient) (*RedisProducer, error) {
	if client == nil {
		return nil, gerror.New("Redis Stream command client 未初始化")
	}
	return &RedisProducer{client: client}, nil
}

// Add 将任务就绪事件写入 Stream，并返回 Redis entry ID。
func (p *RedisProducer) Add(ctx context.Context, stream string, event ReadyEvent) (string, error) {
	body, err := event.Encode()
	if err != nil {
		return "", err
	}
	if !validReadyStream(stream) {
		return "", gerror.New("Redis Stream 名称不合法")
	}
	result, err := p.client.Do(ctx, "XADD", stream, "*", "payload", string(body))
	if err != nil {
		return "", gerror.Wrap(err, "写入 Redis Stream 就绪事件失败")
	}
	if result == nil || result.IsNil() || result.String() == "" {
		return "", gerror.New("Redis Stream 未返回 entry ID")
	}
	return result.String(), nil
}
