package sys

import "testing"

func TestTelegramChannelSenderBotId(t *testing.T) {
	tests := []struct {
		name    string
		channel telegramJobChannel
		want    int64
		wantErr bool
	}{
		{name: "first positive bot", channel: telegramJobChannel{BotIdJson: `[0,12,13]`}, want: 12},
		{name: "empty bots", channel: telegramJobChannel{BotIdJson: `[]`}, wantErr: true},
		{name: "invalid bots", channel: telegramJobChannel{BotIdJson: `invalid`}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := telegramChannelSenderBotId(tt.channel)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyCollectTextDeletes(t *testing.T) {
	got := applyCollectTextDeletes("hello 删除 keep 删除", []string{"删除"})
	if got != "hello  keep " {
		t.Fatalf("got %q", got)
	}
}

func TestMatchedAutoDeleteKeywordNormalizesCommonTraditionalText(t *testing.T) {
	got := matchedAutoDeleteKeyword("❌ 录入失败\n过滤原因：图片去重", []string{"录入失敗"})
	if got != "录入失敗" {
		t.Fatalf("got %q", got)
	}
}

func TestTelegramCopyMediaRefFromFileId(t *testing.T) {
	fileId := telegramCopyMediaFileId("4369206706", 123)
	got, ok := telegramCopyMediaRefFromFileId(fileId)
	if !ok {
		t.Fatal("expected copy media ref")
	}
	if got.ChatId != "-1004369206706" || got.MessageId != 123 {
		t.Fatalf("unexpected ref: %#v", got)
	}
}

func TestTelegramCopyMediaRefFromInvalidFileId(t *testing.T) {
	if _, ok := telegramCopyMediaRefFromFileId("gotd:4369206706:123"); ok {
		t.Fatal("gotd file id should not be treated as copy ref")
	}
}

func TestProfileMediaKeepsCopyReferenceWithoutCacheStatus(t *testing.T) {
	media := profileMedia{TgFileId: "copy:-1004478800787:281"}
	if got := media.ValidTgFileIdForHash(""); got != media.TgFileId {
		t.Fatalf("copy reference should not require tg cache status, got %q", got)
	}
}

func TestTelegramMediaItemPriorityPrefersTelegramCache(t *testing.T) {
	invalid := &telegramMediaItem{StoragePath: "storage/cache/missing.jpg"}
	valid := &telegramMediaItem{TgFileId: "AgAC-valid"}
	if telegramMediaItemPriority(valid) <= telegramMediaItemPriority(invalid) {
		t.Fatal("telegram cached media should have higher priority than storage-only media")
	}
}

func TestTelegramMediaItemHasSource(t *testing.T) {
	if telegramMediaItemHasSource(&telegramMediaItem{}) {
		t.Fatal("empty media should not be considered sendable")
	}
	if !telegramMediaItemHasSource(&telegramMediaItem{FileUrl: "https://cdn.example/image.jpg"}) {
		t.Fatal("media URL should be considered sendable")
	}
}
