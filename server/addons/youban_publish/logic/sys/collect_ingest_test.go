package sys

import "testing"

func TestBotCollectChatMatchesPublishChannel(t *testing.T) {
	if !botCollectChatMatchesPublishChannel(-1004475487847, []string{"4475487847"}) {
		t.Fatal("positive configured channel id must match Bot API channel id")
	}
	if !botCollectChatMatchesPublishChannel(-1003719855649, []string{"-1003719855649"}) {
		t.Fatal("normalized configured channel id must match")
	}
	if botCollectChatMatchesPublishChannel(7074948877, []string{"4475487847", "3719855649"}) {
		t.Fatal("private chat must remain collectable")
	}
}
