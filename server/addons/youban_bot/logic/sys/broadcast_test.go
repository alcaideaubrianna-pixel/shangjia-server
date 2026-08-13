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
