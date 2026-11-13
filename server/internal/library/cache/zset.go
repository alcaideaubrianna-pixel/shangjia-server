package cache

import (
	"context"
	"strconv"

	"github.com/gogf/gf/v2/container/gvar"
)

// ZSetAdd 将成员写入 Redis 有序集合。业务层通过该封装访问 ZSET，避免直接依赖 Redis 客户端。
func ZSetAdd(ctx context.Context, key string, score float64, member string) error {
	client, err := RedisClient()
	if err != nil {
		return err
	}
	_, err = client.Do(ctx, "ZADD", key, strconv.FormatFloat(score, 'f', -1, 64), member)
	return err
}

// ZSetRemove 从 Redis 有序集合移除成员。
func ZSetRemove(ctx context.Context, key string, member string) error {
	client, err := RedisClient()
	if err != nil {
		return err
	}
	_, err = client.Do(ctx, "ZREM", key, member)
	return err
}

// ZSetHead 读取有序集合的第一个成员；集合为空时返回空值。
func ZSetHead(ctx context.Context, key string) (string, error) {
	client, err := RedisClient()
	if err != nil {
		return "", err
	}
	value, err := client.Do(ctx, "ZRANGE", key, 0, 0)
	if err != nil || value == nil || value.IsNil() {
		return "", err
	}
	items := value.Array()
	if len(items) == 0 {
		return "", nil
	}
	return gvar.New(items[0]).String(), nil
}
