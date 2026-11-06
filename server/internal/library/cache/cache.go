// Package cache
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package cache

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gfile"
	"hotgo/internal/library/cache/file"
)

// RedisClient 返回基础设施层 Redis 客户端。
// 只有 queue/cache 等基础设施适配器允许使用；业务逻辑应继续通过 cache.Instance()。
func RedisClient() (*gredis.Redis, error) {
	if strings.ToLower(strings.TrimSpace(g.Cfg().MustGet(context.Background(), "cache.adapter").String())) != "redis" {
		return nil, gerror.New("当前缓存适配器不是 Redis")
	}
	return g.Redis(), nil
}

// cache 缓存驱动
var cache *gcache.Cache

// Instance 缓存实例
func Instance() *gcache.Cache {
	if cache == nil {
		panic("cache uninitialized.")
	}
	return cache
}

func Initialized() bool {
	return cache != nil
}

// SetAdapter 设置缓存适配器
func SetAdapter(ctx context.Context) {
	var adapter gcache.Adapter

	switch g.Cfg().MustGet(ctx, "cache.adapter").String() {
	case "redis":
		adapter = gcache.NewAdapterRedis(g.Redis())
	case "file":
		fileDir := g.Cfg().MustGet(ctx, "cache.fileDir").String()
		if fileDir == "" {
			g.Log().Fatal(ctx, "file path must be configured for file caching.")
			return
		}

		if !gfile.Exists(fileDir) {
			if err := gfile.Mkdir(fileDir); err != nil {
				g.Log().Fatalf(ctx, "failed to create the cache directory. procedure, err:%+v", err)
				return
			}
		}
		adapter = file.NewAdapterFile(fileDir)
	default:
		adapter = gcache.NewAdapterMemory()
	}

	// 数据库缓存，默认和通用缓冲驱动一致，如果你不想使用默认的，可以自行调整
	g.DB().GetCache().SetAdapter(adapter)

	// 通用缓存
	cache = gcache.New()
	cache.SetAdapter(adapter)
}
