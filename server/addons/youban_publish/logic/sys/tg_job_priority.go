package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) telegramJobPriority(job telegramJobRecord) int {
	return telegramJobPriorityValue(job)
}

func isTelegramUrgentJob(job telegramJobRecord) bool {
	return telegramJobPriorityValue(job) <= tgJobPriorityUrgent
}

func telegramJobPriorityValue(job telegramJobRecord) int {
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if isManualProfilePublishOperation(operationNo) ||
		strings.HasPrefix(operationNo, "ai-republish:") ||
		strings.HasPrefix(operationNo, "quick_push:") ||
		strings.HasPrefix(operationNo, "message_push:") {
		return tgJobPriorityUrgent
	}
	if strings.HasPrefix(operationNo, "full_push:") || strings.HasPrefix(operationNo, "cycle_batch:") {
		return tgJobPriorityBulk
	}
	if job.Priority > 0 && job.Priority != 100 {
		return job.Priority
	}
	return tgJobPriorityDefault
}

func telegramQueueNameByPriority(priority int) string {
	switch {
	case priority <= tgJobPriorityUrgent:
		return tgQueueNameUrgent
	case priority >= tgJobPriorityBulk:
		return tgQueueNameBulk
	default:
		return tgQueueNameDefault
	}
}

const defaultTelegramPublishQueueShards = 16

func telegramPublishQueueShardCount(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.publishQueueShards", defaultTelegramPublishQueueShards).Int()
	if value < 1 {
		return 1
	}
	if value > 64 {
		return 64
	}
	return value
}

func telegramQueueNameByPriorityAndChannel(ctx context.Context, priority int, channelId int64) string {
	baseName := telegramQueueNameByPriority(priority)
	if channelId <= 0 {
		return baseName
	}
	shardCount := int64(telegramPublishQueueShardCount(ctx))
	shard := channelId % shardCount
	if shard < 0 {
		shard += shardCount
	}
	return fmt.Sprintf("%s_%d", baseName, shard)
}

func telegramPublishForegroundQueueWeights(ctx context.Context) map[string]int {
	shardCount := telegramPublishQueueShardCount(ctx)
	weights := make(map[string]int, shardCount*2+2)
	for _, item := range []struct {
		baseName string
		weight   int
	}{
		{baseName: tgQueueNameUrgent, weight: 8},
		{baseName: tgQueueNameDefault, weight: 4},
	} {
		weights[item.baseName] = 1
		for shard := 0; shard < shardCount; shard++ {
			weights[fmt.Sprintf("%s_%d", item.baseName, shard)] = item.weight
		}
	}
	return weights
}

func telegramPublishBulkQueueWeights(ctx context.Context) map[string]int {
	shardCount := telegramPublishQueueShardCount(ctx)
	weights := map[string]int{tgQueueNameBulk: 1}
	for shard := 0; shard < shardCount; shard++ {
		weights[fmt.Sprintf("%s_%d", tgQueueNameBulk, shard)] = 1
	}
	return weights
}

func telegramPublishBulkConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.publishBulkConcurrency", 4).Int()
	if value < 1 {
		return 1
	}
	if value > 16 {
		return 16
	}
	return value
}
