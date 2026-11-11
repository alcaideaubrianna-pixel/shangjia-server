// Package ormcache 适配 GoFrame ORM 元数据缓存，不改变业务缓存和业务查询语义。
package ormcache

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gcache"
)

// metadataAdapter 复用已有缓存，仅让元数据查询在共享缓存写锁之外执行。
type metadataAdapter struct{ gcache.Adapter }

// GetOrSetFuncLock 使用 GoFrame 自带的无锁回调加载，写入和并发读取仍由底层缓存保证安全。
// 冷启动可能并行读取同一份不可变表结构，避免数据库连接池与缓存锁形成循环等待。
func (a *metadataAdapter) GetOrSetFuncLock(ctx context.Context, key any, load gcache.Func, duration time.Duration) (*gvar.Var, error) {
	return a.Adapter.GetOrSetFunc(ctx, key, load, duration)
}

// EnableNonBlockingMetadata 在启动 Worker 前启用，复用原适配器及已有缓存值，重复调用无副作用。
// 只能在启动阶段调用，不在已运行的 Worker 上切换适配器。
func EnableNonBlockingMetadata(db gdb.DB) {
	metadata := db.GetCore().GetInnerMemCache()
	if _, enabled := metadata.GetAdapter().(*metadataAdapter); !enabled {
		metadata.SetAdapter(&metadataAdapter{Adapter: metadata.GetAdapter()})
	}
}
