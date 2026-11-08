package profilecycle

import (
	"context"
	"time"
)

// Store 是资料级循环批次持久化适配接口。
type Store interface {
	LoadProfile(context.Context, string) (Profile, error)
	SavePlan(context.Context, *RunPlan) error
}

// Orchestrator 负责 ZSET 到期索引和 PostgreSQL 资料批次之间的交接。
type Orchestrator struct {
	Index DueIndex
	Store Store
}

// ScheduleDue 批量处理到期 run；只有 SavePlan 成功后才移除 ZSET，失败可安全重试。
func (o *Orchestrator) ScheduleDue(ctx context.Context, now time.Time, limit int) (int, error) {
	if o == nil || o.Index == nil || o.Store == nil {
		return 0, ErrInvalidOrchestrator
	}
	keys, err := o.Index.PopDue(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, key := range keys {
		profile, loadErr := o.Store.LoadProfile(ctx, key)
		if loadErr != nil {
			return processed, loadErr
		}
		plan := BuildPlan(profile, key, now)
		if plan == nil {
			if removeErr := o.Index.Remove(ctx, key); removeErr != nil {
				return processed, removeErr
			}
			processed++
			continue
		}
		if err = o.Store.SavePlan(ctx, plan); err != nil {
			return processed, err
		}
		if err = o.Index.Remove(ctx, key); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
