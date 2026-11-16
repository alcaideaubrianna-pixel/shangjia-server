package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

// telegramLegacyPublishQueuesEnabled 迁移期默认排空旧队列；全量新引擎且明确关闭后不再轮询旧分片。
func telegramLegacyPublishQueuesEnabled(ctx context.Context) bool {
	return !globalStreamPushEngineEnabled() || g.Cfg().MustGet(ctx, "youbanPublish.queue.drainLegacyQueues", true).Bool()
}

// telegramPublishExecutionQueues 返回发送执行队列，保留基础队列承接历史下架重试兼容任务。
func telegramPublishExecutionQueues(ctx context.Context) map[string]int {
	// One execution server owns every send lane. Keep legacy queue names in the
	// map during migration so existing Redis tasks remain consumable, while new
	// work uses the same manual/send/bulk registration in every environment.
	queues := telegramPublishForegroundQueueWeightsWithUnifiedLane(ctx)
	for name, weight := range telegramPublishBulkQueueWeights(ctx) {
		queues[name] = weight
	}
	queues[channelDispatchQueueManual] = 8
	return queues
}

// channelDispatchQueueSend 是统一发送任务队列名称。
const channelDispatchQueueSend = "youban_publish_push_send"

// channelDispatchQueueManual 独立承接手动/AI上架，避免被循环和历史任务拖住。
const channelDispatchQueueManual = "youban_publish_push_manual"

// channelDispatchQueueDelete 是独立删除任务队列名称。
const channelDispatchQueueDelete = "youban_publish_push_delete"
const (
	// channelDispatchQueueDeleteManual 用于手动上架触发的删除。
	channelDispatchQueueDeleteManual = "youban_publish_push_delete_manual"
	// channelDispatchQueueDeleteKeyword 用于关键字匹配删除。
	channelDispatchQueueDeleteKeyword = "youban_publish_push_delete_keyword"
	// channelDispatchQueueDeleteCycle 用于循环翻新删除。
	channelDispatchQueueDeleteCycle = "youban_publish_push_delete_cycle"
)

// channelDispatchLane 表示任务所属的执行通道。
type channelDispatchLane uint8

const (
	// channelDispatchLaneSend 表示消息发送通道。
	channelDispatchLaneSend channelDispatchLane = iota
	// channelDispatchLaneDelete 表示消息删除通道。
	channelDispatchLaneDelete
)

// channelDispatchQueueName 返回统一调度通道对应的 Asynq 队列名称。
func channelDispatchQueueName(lane channelDispatchLane) string {
	if lane == channelDispatchLaneDelete {
		return channelDispatchQueueDelete
	}
	return channelDispatchQueueSend
}

// channelDispatchDeleteQueueName 根据删除优先级返回独立删除队列。
func channelDispatchDeleteQueueName(priority channelDispatchPriority) string {
	switch priority {
	case channelPriorityManualDelete:
		return channelDispatchQueueDeleteManual
	case channelPriorityCycleDelete:
		return channelDispatchQueueDeleteCycle
	default:
		return channelDispatchQueueDeleteKeyword
	}
}

// telegramPublishForegroundQueueWeightsWithUnifiedLane 在保留旧队列的同时注册统一发送队列。
func telegramPublishForegroundQueueWeightsWithUnifiedLane(ctx context.Context) map[string]int {
	queues := telegramPublishForegroundQueueWeights(ctx)
	queues[channelDispatchQueueSend] = 1
	return queues
}

// queueNameForPushEngine 根据账户引擎选择发送队列，旧引擎保留调用方已有队列。
func queueNameForPushEngine(engine, legacyQueue string) string {
	if engine == pushEngineUnified || engine == pushEngineUnifiedStream {
		return channelDispatchQueueName(channelDispatchLaneSend)
	}
	return legacyQueue
}

// channelDispatchPriority 定义频道内任务优先级，数值越小越优先。
type channelDispatchPriority int

const (
	// channelPriorityManualDelete 表示手动下架触发的旧消息删除。
	channelPriorityManualDelete channelDispatchPriority = iota
	// channelPriorityKeywordDelete 表示关键字规则触发的删除。
	channelPriorityKeywordDelete
	// channelPriorityCycleDelete 表示循环翻新触发的旧消息删除。
	channelPriorityCycleDelete
)

const telegramCleanupManualReservedSlotsDefault = 1

// normalizeTelegramCleanupManualReservedSlots 限制手动删除的频道保底槽位。
// 至少保留一个槽位给自动删除，避免配置错误导致自动删除永久无法执行。
func normalizeTelegramCleanupManualReservedSlots(configured, total int) int {
	if total <= 1 {
		return 0
	}
	if configured < 0 {
		return 0
	}
	if configured >= total {
		return total - 1
	}
	return configured
}

// telegramCleanupSlotLimitForPriority 返回任务可使用的频道清理槽位数量。
// 手动删除可以使用全部槽位；关键字和循环删除让出配置的手动保底槽位。
func telegramCleanupSlotLimitForPriority(priority channelDispatchPriority, total, reserved int) int {
	if total <= 0 {
		return 0
	}
	if priority == channelPriorityManualDelete {
		return total
	}
	reserved = normalizeTelegramCleanupManualReservedSlots(reserved, total)
	limit := total - reserved
	if limit < 1 {
		return 1
	}
	return limit
}

// telegramCleanupSlotLimit 使用配置返回当前任务可使用的频道清理槽位数量。
func telegramCleanupSlotLimit(ctx context.Context, priority channelDispatchPriority) int {
	reserved := g.Cfg().MustGet(ctx, "youbanPublish.queue.cleanupManualReservedSlots", telegramCleanupManualReservedSlotsDefault).Int()
	return telegramCleanupSlotLimitForPriority(channelDispatchPriority(priority), telegramCleanupPerChannelConcurrency, reserved)
}

// telegramCleanupPriority 按本次清理请求分类，不让历史发布优先级降低手动下架优先级。
func telegramCleanupPriority(jobPriority int, cleanupOperationNo string) channelDispatchPriority {
	if strings.TrimSpace(cleanupOperationNo) != "" || jobPriority <= tgJobPriorityInteractive {
		return channelPriorityManualDelete
	}
	return channelPriorityCycleDelete
}

// telegramCleanupExecutionPriority 优先使用实际消费队列，兼容没有请求上下文的恢复任务。
func telegramCleanupExecutionPriority(ctx context.Context, job telegramJobRecord) channelDispatchPriority {
	if queue, ok := asynq.GetQueueName(ctx); ok {
		switch queue {
		case channelDispatchQueueDeleteManual:
			return channelPriorityManualDelete
		case channelDispatchQueueDeleteKeyword:
			return channelPriorityKeywordDelete
		case channelDispatchQueueDeleteCycle:
			return channelPriorityCycleDelete
		}
	}
	return telegramCleanupPriority(job.Priority, "")
}
