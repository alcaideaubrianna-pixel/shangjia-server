package runtime

import (
	"testing"

	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestValidAccountHistoryPageRequest(t *testing.T) {
	tests := []struct {
		name string
		peer tg.InputPeerClass
		want bool
	}{
		{name: "basic group", peer: &tg.InputPeerChat{ChatID: 5345385902}, want: true},
		{name: "channel", peer: &tg.InputPeerChannel{ChannelID: 123, AccessHash: 456}, want: true},
		{name: "channel without access hash", peer: &tg.InputPeerChannel{ChannelID: 123}, want: false},
		{name: "unsupported peer", peer: &tg.InputPeerUser{UserID: 123, AccessHash: 456}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &sysin.AccountHistoryPageRequest{Peer: tt.peer, Limit: 100}
			if got := validAccountHistoryPageRequest(request); got != tt.want {
				t.Fatalf("validAccountHistoryPageRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}
