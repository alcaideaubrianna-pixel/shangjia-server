package sys

import (
	"errors"
	"testing"
)

func TestShouldMarkBotOffline(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unauthorized token", err: errors.New("unauthorized, Unauthorized"), want: true},
		{name: "invalid bot token", err: errors.New("bot token is invalid"), want: true},
		{name: "invalid token not found", err: errors.New("not found, Not Found"), want: true},
		{name: "user blocked bot", err: errors.New("forbidden, Forbidden: bot was blocked by the user"), want: false},
		{name: "chat not found", err: errors.New("bad request, Bad Request: chat not found"), want: false},
		{name: "message not found", err: errors.New("bad request, Bad Request: message to delete not found"), want: false},
		{name: "permission forbidden", err: errors.New("forbidden, Forbidden: bot is not a member of the channel chat"), want: false},
		{name: "rate limited", err: errors.New("too many requests, retry after 30"), want: false},
		{name: "timeout", err: errors.New("context deadline exceeded"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldMarkBotOffline(test.err); got != test.want {
				t.Fatalf("shouldMarkBotOffline() = %v, want %v, err: %v", got, test.want, test.err)
			}
		})
	}
}
