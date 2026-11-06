package streamgray

import "testing"

// TestReadyEventRoundTrip 验证就绪事件编码和解码保持幂等字段。
func TestReadyEventRoundTrip(t *testing.T) {
	want := ReadyEvent{JobID: 123, ChannelID: 456, Priority: 90, QueueName: "publish"}
	body, err := want.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeReadyEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

// TestReadyEventValidation 验证无效事件不会进入传输适配层。
func TestReadyEventValidation(t *testing.T) {
	if _, err := (ReadyEvent{ChannelID: 1}).Encode(); err == nil {
		t.Fatal("missing job id must fail")
	}
	if _, err := DecodeReadyEvent([]byte(`{"job_id":1}`)); err == nil {
		t.Fatal("missing channel id must fail")
	}
}
