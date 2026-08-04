package sys

import "testing"

func TestTelegramDeleteMessageBatchesGroupsByChatAndLimit(t *testing.T) {
	messages := []telegramDeleteMessage{
		{Id: 1, TargetChatId: "123", MessageId: 11},
		{Id: 2, TargetChatId: "-100456", MessageId: 21},
		{Id: 3, TargetChatId: "123", MessageId: 12},
		{Id: 4, TargetChatId: "123", MessageId: 13},
		{Id: 0, TargetChatId: "123", MessageId: 14},
	}

	batches := telegramDeleteMessageBatches(messages, 2)
	if len(batches) != 3 {
		t.Fatalf("unexpected batch count: %d", len(batches))
	}
	if batches[0].chatId != "123" || len(batches[0].messages) != 2 {
		t.Fatalf("unexpected first batch: %#v", batches[0])
	}
	if batches[1].chatId != "-100456" || len(batches[1].messages) != 1 {
		t.Fatalf("unexpected second batch: %#v", batches[1])
	}
	if batches[2].chatId != "123" || len(batches[2].messages) != 1 {
		t.Fatalf("unexpected third batch: %#v", batches[2])
	}
}
