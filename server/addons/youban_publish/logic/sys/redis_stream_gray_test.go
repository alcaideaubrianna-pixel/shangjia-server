package sys

import (
	"encoding/json"
	"testing"
)

// TestRedisStreamReadyEventEncoding 验证就绪事件只包含任务调度所需字段。
func TestRedisStreamReadyEventEncoding(t *testing.T) {
	body, err := encodeRedisStreamReadyEvent(redisStreamReadyEvent{
		JobID: 123, ChannelID: 456, Priority: 90, QueueName: channelDispatchQueueSend,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got redisStreamReadyEvent
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.JobID != 123 || got.ChannelID != 456 || got.Priority != 90 || got.QueueName != channelDispatchQueueSend {
		t.Fatalf("event = %+v", got)
	}
}

// TestRedisStreamReadyEventValidation 验证无效事件不会进入灰度传输层。
func TestRedisStreamReadyEventValidation(t *testing.T) {
	if _, err := encodeRedisStreamReadyEvent(redisStreamReadyEvent{ChannelID: 456}); err == nil {
		t.Fatal("missing job id must be rejected")
	}
	if !redisStreamPushEngineEnabled(pushEngineUnifiedStream) {
		t.Fatal("stream engine must be recognized")
	}
	if redisStreamPushEngineEnabled(pushEngineUnified) {
		t.Fatal("legacy unified engine must not be treated as stream transport")
	}
}
