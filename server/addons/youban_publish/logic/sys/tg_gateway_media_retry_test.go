package sys

import "testing"

func TestTelegramMediaSetForGatewayRetryKeepsGroupAndForcesUpload(t *testing.T) {
	media := []*telegramMediaItem{
		{Id: 1, FileUrl: "https://cdn.example/1.jpg", TgFileId: "file-1"},
		{Id: 2, FileUrl: "https://cdn.example/2.jpg", TgFileId: "file-2"},
	}
	got := telegramMediaSetForGatewayRetry(telegramJobRecord{
		RetryCount:   1,
		ErrorMessage: "error response from telegram for method sendMediaGroup, 502 Bad Gateway",
	}, media)
	if len(got) != len(media) {
		t.Fatalf("media group size changed: got %d want %d", len(got), len(media))
	}
	for index, item := range got {
		if item.Id != media[index].Id || !item.ForceUpload || item.TgFileId != "" {
			t.Fatalf("media %d did not retain order with forced upload: %+v", index, item)
		}
	}
	if media[0].ForceUpload || media[0].TgFileId != "file-1" {
		t.Fatalf("source media mutated: %+v", media[0])
	}
}

func TestTelegramMediaSetForGatewayRetryLeavesFirstAttemptUnchanged(t *testing.T) {
	media := []*telegramMediaItem{{Id: 1, FileUrl: "https://cdn.example/1.jpg"}}
	got := telegramMediaSetForGatewayRetry(telegramJobRecord{
		ErrorMessage: "error response from telegram for method sendMediaGroup, 502 Bad Gateway",
	}, media)
	if got[0] != media[0] || got[0].ForceUpload {
		t.Fatalf("first attempt should retain primary media mode: %+v", got[0])
	}
}
