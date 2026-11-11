package sys

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

// telegramLegacyPublishQueuesEnabled 迁移期默认排空旧队列；全量新引擎且明确关闭后不再轮询旧分片。
func telegramLegacyPublishQueuesEnabled(ctx context.Context) bool {
	return !globalStreamPushEngineEnabled() || g.Cfg().MustGet(ctx, "youbanPublish.queue.drainLegacyQueues", true).Bool()
}

// telegramPublishExecutionQueues 返回发送执行队列，保留基础队列承接历史下架重试兼容任务。
func telegramPublishExecutionQueues(ctx context.Context) map[string]int {
	if telegramLegacyPublishQueuesEnabled(ctx) {
		return telegramPublishForegroundQueueWeightsWithUnifiedLane(ctx)
	}
	return map[string]int{channelDispatchQueueSend: 1, tgQueueNameDefault: 1}
}

// channelDispatchQueueSend 是统一发送任务队列名称。
const channelDispatchQueueSend = "youban_publish_push_send"

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
	// channelPriorityManualDelete 表示手动上架触发的旧消息删除。
	channelPriorityManualDelete channelDispatchPriority = iota
	// channelPriorityKeywordDelete 表示关键字规则触发的删除。
	channelPriorityKeywordDelete
	// channelPriorityCycleDelete 表示循环翻新触发的旧消息删除。
	channelPriorityCycleDelete
)
