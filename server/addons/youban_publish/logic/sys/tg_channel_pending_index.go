package sys

import (
	"context"
	"fmt"
	"strconv"

	"hotgo/internal/library/cache"
)

// tgChannelPendingIndex 是频道待投递 Job 的 Redis 高速索引。
// PostgreSQL 仍是事实记录，索引丢失时由 Outbox 恢复流程重新写入。
type tgChannelPendingIndex struct{}

func tgChannelPendingKey(tenantID, channelID int64) string {
	return fmt.Sprintf("youban_publish:tg:channel_pending:%d:%d", tenantID, channelID)
}

// add 将 Job 按优先级和创建序号写入频道 ZSET；分数越小越优先。
func (tgChannelPendingIndex) add(ctx context.Context, tenantID, channelID, jobID int64, priority int) error {
	if tenantID <= 0 || channelID <= 0 || jobID <= 0 {
		return nil
	}
	if priority <= 0 {
		priority = 50
	}
	// 组合分数保留优先级顺序，同时用 Job ID 保证同优先级 FIFO。
	score := float64(priority)*1_000_000_000_000 + float64(jobID)
	return cache.ZSetAdd(ctx, tgChannelPendingKey(tenantID, channelID), score, strconv.FormatInt(jobID, 10))
}

func (tgChannelPendingIndex) remove(ctx context.Context, tenantID, channelID, jobID int64) error {
	if tenantID <= 0 || channelID <= 0 || jobID <= 0 {
		return nil
	}
	return cache.ZSetRemove(ctx, tgChannelPendingKey(tenantID, channelID), strconv.FormatInt(jobID, 10))
}

func (tgChannelPendingIndex) head(ctx context.Context, tenantID, channelID int64) (int64, error) {
	if tenantID <= 0 || channelID <= 0 {
		return 0, nil
	}
	value, err := cache.ZSetHead(ctx, tgChannelPendingKey(tenantID, channelID))
	if err != nil || value == "" {
		return 0, err
	}
	jobID, _ := strconv.ParseInt(value, 10, 64)
	return jobID, nil
}
