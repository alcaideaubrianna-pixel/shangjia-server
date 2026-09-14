package cache

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// HashGetMany reads selected fields without materializing the complete Redis hash.
func HashGetMany(ctx context.Context, key string, fields []string) (gvar.Vars, error) {
	if len(fields) == 0 {
		return gvar.Vars{}, nil
	}
	return g.Redis().HMGet(ctx, key, fields...)
}

// HashScan incrementally reads a Redis hash. COUNT is a batch-size hint from Redis.
func HashScan(ctx context.Context, key string, cursor uint64, count int) (uint64, map[string]*gvar.Var, error) {
	result, err := g.Redis().Do(ctx, "HSCAN", key, cursor, "COUNT", count)
	if err != nil {
		return 0, nil, err
	}
	parts := result.Array()
	if len(parts) != 2 {
		return 0, map[string]*gvar.Var{}, nil
	}
	next := gconv.Uint64(parts[0])
	items := gvar.New(parts[1]).Array()
	fields := make(map[string]*gvar.Var, len(items)/2)
	for index := 0; index+1 < len(items); index += 2 {
		fields[gconv.String(items[index])] = gvar.New(items[index+1])
	}
	return next, fields, nil
}

// HashSetMany writes selected fields and refreshes the hash lifetime.
func HashSetMany(ctx context.Context, key string, fields map[string]any, lifetime time.Duration) error {
	if len(fields) == 0 {
		return nil
	}
	if _, err := g.Redis().HSet(ctx, key, fields); err != nil {
		return err
	}
	if lifetime > 0 {
		_, err := g.Redis().PExpire(ctx, key, lifetime.Milliseconds())
		return err
	}
	return nil
}
