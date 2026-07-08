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

func TestCollectStringListSupportsObjects(t *testing.T) {
	got := collectStringList(`[{"label":"A"},{"text":"B"},{"value":"A"}]`)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("unexpected list: %#v", got)
	}
}
