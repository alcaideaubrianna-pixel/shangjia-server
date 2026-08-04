package sys

import "testing"

func TestChannelQueueClearStatusesExcludeSending(t *testing.T) {
	for _, status := range channelQueueClearStatuses() {
		if status == "sending" {
			t.Fatal("queue clearing must not supersede an in-flight Telegram request")
		}
	}
}
