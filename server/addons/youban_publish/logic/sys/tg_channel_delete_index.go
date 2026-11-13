package sys

import (
	"context"
	"fmt"
	"strconv"

	"hotgo/internal/library/cache"
)

// tgChannelDeleteIndex 是频道删除任务的 Redis 高速索引。
//
// PostgreSQL 中的 TG Job/Outbox 仍然是删除事实记录，Redis 只保存待删除
// Job 或自动删除消息的排序索引。索引丢失时由 Outbox/自动删除重试重新写入，
// 因此不会因为 Redis 故障丢失删除任务。
type tgChannelDeleteIndex struct{}

func tgChannelDeleteKey(tenantID, channelID int64) string {
	return fmt.Sprintf("youban_publish:tg:channel_delete:%d:%d", tenantID, channelID)
}

func (tgChannelDeleteIndex) addJob(ctx context.Context, tenantID, channelID, jobID int64, priority channelDispatchPriority) error {
	if tenantID <= 0 || channelID <= 0 || jobID <= 0 {
		return nil
	}
	// 分数先按删除优先级排序，再按 Job ID 保持同优先级 FIFO。
	score := float64(priority)*1_000_000_000_000 + float64(jobID)
	return cache.ZSetAdd(ctx, tgChannelDeleteKey(tenantID, channelID), score, "job:"+strconv.FormatInt(jobID, 10))
}

func (tgChannelDeleteIndex) removeJob(ctx context.Context, tenantID, channelID, jobID int64) error {
	if tenantID <= 0 || channelID <= 0 || jobID <= 0 {
		return nil
	}
	return cache.ZSetRemove(ctx, tgChannelDeleteKey(tenantID, channelID), "job:"+strconv.FormatInt(jobID, 10))
}

func (tgChannelDeleteIndex) addAutoDelete(ctx context.Context, tenantID, channelID int64, messageID int, priority channelDispatchPriority) error {
	if tenantID <= 0 || channelID <= 0 || messageID <= 0 {
		return nil
	}
	score := float64(priority)*1_000_000_000_000 + float64(messageID)
	return cache.ZSetAdd(ctx, tgChannelDeleteKey(tenantID, channelID), score, fmt.Sprintf("message:%d", messageID))
}

func (tgChannelDeleteIndex) removeAutoDelete(ctx context.Context, tenantID, channelID int64, messageID int) error {
	if tenantID <= 0 || channelID <= 0 || messageID <= 0 {
		return nil
	}
	return cache.ZSetRemove(ctx, tgChannelDeleteKey(tenantID, channelID), fmt.Sprintf("message:%d", messageID))
}

func (tgChannelDeleteIndex) head(ctx context.Context, tenantID, channelID int64) (string, error) {
	if tenantID <= 0 || channelID <= 0 {
		return "", nil
	}
	return cache.ZSetHead(ctx, tgChannelDeleteKey(tenantID, channelID))
}
