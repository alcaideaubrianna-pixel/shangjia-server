package sys

import (
	"errors"
	"testing"
)

func TestIsBroadcastBlocked(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{message: "Forbidden: bot was blocked by the user", want: true},
		{message: "Bad Request: chat not found", want: true},
		{message: "Forbidden: user is deactivated", want: true},
		{message: "Too Many Requests: retry_after 3", want: false},
	}
	for _, test := range tests {
		if got := isBroadcastBlocked(errors.New(test.message)); got != test.want {
			t.Fatalf("isBroadcastBlocked(%q)=%v, want %v", test.message, got, test.want)
		}
	}
}

func TestUniqueBroadcastRecipients(t *testing.T) {
	rows := []*broadcastRecipient{
		{BotId: 1, ChatId: "100", TelegramUserId: "100"},
		{BotId: 2, ChatId: "100", TelegramUserId: "100"},
		{BotId: 1, ChatId: "200", TelegramUserId: "200"},
	}
	got := uniqueBroadcastRecipients(rows)
	if len(got) != 2 || got[0].BotId != 1 || got[1].TelegramUserId != "200" {
		t.Fatalf("unexpected recipients: %+v", got)
	}
}

func TestPositiveUniqueInt64s(t *testing.T) {
	got := positiveUniqueInt64s([]int64{0, 1, 1, -2, 3})
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("unexpected ids: %v", got)
	}
}

func TestBotIdsFromRows(t *testing.T) {
	if got := botIdsFromRows([]*broadcastBotIdRow{{Id: 1}}, false); len(got) != 1 || got[0] != 1 {
		t.Fatalf("unexpected bot ids: %v", got)
	}
	if got := botIdsFromRows([]*broadcastBotIdRow{{BotId: 1}}, true); len(got) != 1 || got[0] != 1 {
		t.Fatalf("unexpected relation bot ids: %v", got)
	}
}
