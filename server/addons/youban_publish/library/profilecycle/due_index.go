package profilecycle

import (
	"context"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
)

const DueIndexKey = "youban_publish:profile_cycle:due"

// RedisCommandClient 是 ZSET 到期索引所需的最小 Redis 接口。
type RedisCommandClient interface {
	Do(context.Context, string, ...any) (*gvar.Var, error)
}

// RedisDueIndex 使用 ZSET 保存资料级 run 的到期时间。
type RedisDueIndex struct {
	client RedisCommandClient
	key    string
}

func NewRedisDueIndex(client RedisCommandClient, key string) (*RedisDueIndex, error) {
	if client == nil || key == "" {
		return nil, gerror.New("profile cycle due index 参数无效")
	}
	return &RedisDueIndex{client: client, key: key}, nil
}

// Add 写入或更新 run 的到期时间。
func (i *RedisDueIndex) Add(ctx context.Context, runKey string, dueAt time.Time) error {
	if runKey == "" {
		return gerror.New("profile cycle run key 不能为空")
	}
	_, err := i.client.Do(ctx, "ZADD", i.key, dueAt.UnixMilli(), runKey)
	return err
}

// PopDue 读取到期 run；调用方成功落库后应调用 Remove。
func (i *RedisDueIndex) PopDue(ctx context.Context, now time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, gerror.New("profile cycle due index limit 无效")
	}
	value, err := i.client.Do(ctx, "ZRANGEBYSCORE", i.key, "-inf", strconv.FormatInt(now.UnixMilli(), 10), "LIMIT", 0, limit)
	if err != nil {
		return nil, err
	}
	if value == nil || value.IsNil() {
		return nil, nil
	}
	items := value.Array()
	keys := make([]string, 0, len(items))
	for _, item := range items {
		key := gvar.New(item).String()
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (i *RedisDueIndex) Remove(ctx context.Context, runKey string) error {
	if runKey == "" {
		return nil
	}
	_, err := i.client.Do(ctx, "ZREM", i.key, runKey)
	return err
}
