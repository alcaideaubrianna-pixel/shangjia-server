package cache

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

const hashSetManyBatchSize = 500

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
	fieldNames := make([]string, 0, len(fields))
	for field := range fields {
		fieldNames = append(fieldNames, field)
	}
	sort.Strings(fieldNames)
	for start := 0; start < len(fieldNames); start += hashSetManyBatchSize {
		end := start + hashSetManyBatchSize
		if end > len(fieldNames) {
			end = len(fieldNames)
		}
		args := make([]any, 0, 1+(end-start)*2)
		args = append(args, key)
		for _, field := range fieldNames[start:end] {
			args = append(args, field, fields[field])
		}
		if _, err := g.Redis().Do(ctx, "HSET", args...); err != nil {
			return err
		}
	}
	if lifetime > 0 {
		result, err := g.Redis().PExpire(ctx, key, lifetime.Milliseconds())
		if err != nil {
			return err
		}
		if result == 0 {
			return fmt.Errorf("redis hash expiry target is missing: key=%s", key)
		}
	}
	length, err := HashLen(ctx, key)
	if err != nil {
		return err
	}
	if length <= 0 {
		return fmt.Errorf("redis hash write produced no fields: key=%s", key)
	}
	return nil
}

// HashLen returns the number of fields persisted in a Redis hash.
func HashLen(ctx context.Context, key string) (int64, error) {
	result, err := g.Redis().Do(ctx, "HLEN", key)
	if err != nil {
		return 0, err
	}
	return result.Int64(), nil
}
