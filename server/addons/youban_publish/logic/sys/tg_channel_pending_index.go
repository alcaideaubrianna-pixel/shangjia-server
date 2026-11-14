package sys

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gogf/gf/v2/frame/g"
)

// tgChannelPendingIndex 是频道待投递 Job 的 Redis 高速索引。
// PostgreSQL 仍是事实记录，索引丢失时由 Outbox 恢复流程重新写入。
type tgChannelPendingIndex struct{}

func tgChannelPendingKey(tenantID, channelID int64) string {
	// tenantID is retained for source compatibility; channel queues are keyed
	// by the publish channel only and use the single canonical namespace.
	return telegramChannelQueueKey(channelID)
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
	_, err := g.Redis().Do(ctx, "ZADD", tgChannelPendingKey(tenantID, channelID), score, strconv.FormatInt(jobID, 10))
	return err
}

func (tgChannelPendingIndex) remove(ctx context.Context, tenantID, channelID, jobID int64) error {
	if tenantID <= 0 || channelID <= 0 || jobID <= 0 {
		return nil
	}
	_, err := g.Redis().Do(ctx, "ZREM", tgChannelPendingKey(tenantID, channelID), strconv.FormatInt(jobID, 10))
	return err
}

func (tgChannelPendingIndex) head(ctx context.Context, tenantID, channelID int64) (int64, error) {
	if tenantID <= 0 || channelID <= 0 {
		return 0, nil
	}
	value, err := g.Redis().Do(ctx, "ZRANGE", tgChannelPendingKey(tenantID, channelID), 0, 0)
	if err != nil || value.IsNil() {
		return 0, err
	}
	items := value.Array()
	if len(items) == 0 {
		return 0, nil
	}
	jobID, _ := strconv.ParseInt(fmt.Sprint(items[0]), 10, 64)
	return jobID, nil
}
