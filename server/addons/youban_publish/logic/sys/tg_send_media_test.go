package sys

import "testing"

func TestValidateTelegramMediaPurpose(t *testing.T) {
	tests := []struct {
		name    string
		purpose string
		media   []*telegramMediaItem
		wantErr bool
	}{
		{name: "display only", purpose: "display", media: []*telegramMediaItem{{Purpose: "display"}}},
		{name: "verify only", purpose: "verify", media: []*telegramMediaItem{{Purpose: "verify"}}},
		{name: "mixed purposes", purpose: "display", media: []*telegramMediaItem{{Purpose: "display"}, {Purpose: "verify"}}, wantErr: true},
		{name: "wrong purpose", purpose: "verify", media: []*telegramMediaItem{{Purpose: "display"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTelegramMediaPurpose(test.purpose, test.media)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTelegramMediaPurpose() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestTelegramMediaSetHasCompleteCopyRefs(t *testing.T) {
	complete := []*telegramMediaItem{
		{TgFileId: "copy:123:10"},
		{TgFileId: "copy:123:11"},
	}
	mixed := []*telegramMediaItem{
		{TgFileId: "copy:123:10"},
		{TgFileId: "BAACAgEAA-test"},
	}

	if !telegramMediaSetHasCompleteCopyRefs(complete) {
		t.Fatal("complete copy references should use Telegram group copy")
	}
	if telegramMediaSetHasCompleteCopyRefs(mixed) {
		t.Fatal("mixed copy references must fall back to persistent media upload")
	}
}
