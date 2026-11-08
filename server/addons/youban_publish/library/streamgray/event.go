// Package streamgray 提供发布插件 Redis Stream 灰度传输的隔离抽象。
package streamgray

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

// StreamForOperation 根据任务操作号选择就绪事件流。
func StreamForOperation(operationNo string) string {
	operationNo = strings.ToLower(strings.TrimSpace(operationNo))
	if strings.HasPrefix(operationNo, "cycle_batch:") || strings.HasPrefix(operationNo, "full_push:") {
		return ReadyBulkStream
	}
	return ReadyRealtimeStream
}

const (
	// ReadyRealtimeStream 是实时任务就绪事件流。
	ReadyRealtimeStream = "youban_publish:tg:ready:realtime"
	// ReadyBulkStream 是循环和批量任务就绪事件流。
	ReadyBulkStream = "youban_publish:tg:ready:bulk"
)

// validReadyStream 判断事件是否写入允许的灰度 Stream。
func validReadyStream(stream string) bool {
	return stream == ReadyRealtimeStream || stream == ReadyBulkStream
}

// ReadyEvent 是数据库任务提交后的轻量通知，不承载发送最终状态。
type ReadyEvent struct {
	TenantID  int64  `json:"tenant_id"`
	AccountID int64  `json:"account_id"`
	JobID     int64  `json:"job_id"`
	ChannelID int64  `json:"channel_id"`
	Priority  int    `json:"priority"`
	QueueName string `json:"queue_name"`
}

// Validate 检查就绪事件是否包含消费者投递所需字段。
func (e ReadyEvent) Validate() error {
	if e.TenantID <= 0 || e.AccountID <= 0 || e.JobID <= 0 || e.ChannelID <= 0 {
		return gerror.New("Redis Stream 就绪事件参数无效")
	}
	return nil
}

// Encode 将事件编码为 Stream 消息体。
func (e ReadyEvent) Encode() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

// DecodeReadyEvent 从 Stream 消息体恢复事件。
func DecodeReadyEvent(body []byte) (ReadyEvent, error) {
	var event ReadyEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return event, gerror.Wrap(err, "解析 Redis Stream 就绪事件失败")
	}
	if err := event.Validate(); err != nil {
		return event, err
	}
	return event, nil
}

// Producer 是 Redis Stream 生产端的最小接口，业务层不感知 Redis 客户端。
type Producer interface {
	Add(ctx context.Context, stream string, event ReadyEvent) (string, error)
}

// Consumer 是 Stream 消费端的最小接口，实际实现负责 pending 和 ACK 管理。
type Consumer interface {
	Consume(ctx context.Context, stream string, handler func(context.Context, ReadyEvent) error) error
}

// PublishReadyEvent 统一发布就绪事件，业务层不直接操作 Redis 命令。
func PublishReadyEvent(ctx context.Context, producer Producer, stream string, event ReadyEvent) (string, error) {
	if producer == nil {
		return "", gerror.New("Redis Stream producer 未初始化")
	}
	if !validReadyStream(stream) {
		return "", gerror.New("Redis Stream 名称不合法")
	}
	if err := event.Validate(); err != nil {
		return "", err
	}
	return producer.Add(ctx, stream, event)
}
