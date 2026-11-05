package sys

// channelDispatchQueueSend 是统一发送任务队列名称。
const channelDispatchQueueSend = "youban_publish_push_send"

// channelDispatchQueueDelete 是独立删除任务队列名称。
const channelDispatchQueueDelete = "youban_publish_push_delete"

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

// channelSendPriority 定义上架任务的优先级，数值越小越优先。
type channelSendPriority int

const (
	// channelPriorityManualUpload 表示用户主动上架。
	channelPriorityManualUpload channelSendPriority = iota
	// channelPriorityRealtimeCollect 表示实时采集推送。
	channelPriorityRealtimeCollect
	// channelPriorityAIRepair 表示 AI 巡检修复。
	channelPriorityAIRepair
	// channelPriorityCycle 表示循环翻新。
	channelPriorityCycle
	// channelPriorityBatch 表示批次上架。
	channelPriorityBatch
)
