package sys

import (
	"testing"

	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectHistoryInputPeer(t *testing.T) {
	t.Run("basic group does not require access hash", func(t *testing.T) {
		peer, err := collectHistoryInputPeer(&sysin.ChannelCacheModel{ChannelId: "-5345385902"})
		if err != nil {
			t.Fatalf("collectHistoryInputPeer() error = %v", err)
		}
		chat, ok := peer.(*tg.InputPeerChat)
		if !ok || chat.ChatID != 5345385902 {
			t.Fatalf("collectHistoryInputPeer() = %#v, want InputPeerChat(5345385902)", peer)
		}
	})

	t.Run("channel requires and preserves access hash", func(t *testing.T) {
		peer, err := collectHistoryInputPeer(&sysin.ChannelCacheModel{
			ChannelId: "-100123456", AccessHash: "987654", IsMegagroup: 1,
		})
		if err != nil {
			t.Fatalf("collectHistoryInputPeer() error = %v", err)
		}
		channel, ok := peer.(*tg.InputPeerChannel)
		if !ok || channel.ChannelID != 123456 || channel.AccessHash != 987654 {
			t.Fatalf("collectHistoryInputPeer() = %#v, want InputPeerChannel(123456, 987654)", peer)
		}
	})
}
