package sys

import (
	"context"
	"testing"
)

func TestTelegramJobChannelsRejectsEmptyExplicitSelection(t *testing.T) {
	service := &sSysPublish{}
	_, err := service.telegramJobChannels(context.Background(), nil, []int64{})
	if err != errNoTelegramPublishChannels {
		t.Fatalf("error=%v, want %v", err, errNoTelegramPublishChannels)
	}
}
