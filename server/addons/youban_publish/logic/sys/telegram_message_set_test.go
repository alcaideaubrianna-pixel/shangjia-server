package sys

import (
	"errors"
	"testing"
)

func TestIsTelegramBotRemovedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "kicked", err: errors.New("forbidden: bot was kicked from the channel"), want: true},
		{name: "blocked", err: errors.New("forbidden: bot was blocked by the user"), want: true},
		{name: "chat missing", err: errors.New("bad request: chat not found"), want: true},
		{name: "rate limited", err: errors.New("too many requests: retry after 5"), want: false},
		{name: "permission", err: errors.New("not enough rights to delete messages"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramBotRemovedError(test.err); got != test.want {
				t.Fatalf("isTelegramBotRemovedError() = %v, want %v", got, test.want)
			}
		})
	}
}
