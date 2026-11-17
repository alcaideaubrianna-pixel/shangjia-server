package botnotice

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPreviouslyExportedPayloadsLocalOnly(t *testing.T) {
	path := os.Getenv("YOUBAN_BOT_NOTICE_LOCAL_CASES")
	if path == "" {
		t.Skip("optional local exported payloads")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []json.RawMessage
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	ignored, media := 0, 0
	for _, raw := range cases {
		var update struct {
			ChannelPost map[string]json.RawMessage `json:"channel_post"`
		}
		if err := json.Unmarshal(raw, &update); err != nil {
			t.Fatal(err)
		}
		hasMedia := len(update.ChannelPost["photo"]) > 0 || len(update.ChannelPost["video"]) > 0 || len(update.ChannelPost["document"]) > 0 || len(update.ChannelPost["media_group_id"]) > 0
		if hasMedia {
			media++
		}
		if PlainUpdate(raw) {
			ignored++
			if hasMedia {
				t.Fatal("exported media payload was classified as a receipt")
			}
		}
	}
	t.Logf("local exported payloads=%d, receipts=%d, protected media messages=%d", len(cases), ignored, media)
}

func TestPlainUpdateProtectsMediaAndOrdinaryText(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message map[string]any
		ignore  bool
	}{
		{"duplicate_receipt", map[string]any{"text": "❌ 重复投稿\n检测到短时间内提交的相同内容，请稍后再试"}, true},
		{"success_receipt", map[string]any{"text": "✅ 提交成功"}, true},
		{"verify_label_only", map[string]any{"text": "验证视频"}, true},
		{"ordinary_text", map[string]any{"text": "昵称：本地测试\n年龄：22"}, false},
		{"media_caption", map[string]any{"caption": "验证视频", "video": map[string]any{"file_id": "local-video"}}, false},
		{"photo_with_receipt_text", map[string]any{"text": "✅提交成功", "photo": []any{map[string]any{"file_id": "local-photo"}}}, false},
		{"document", map[string]any{"text": "✅提交成功", "document": map[string]any{"file_id": "local-document"}}, false},
		{"unknown_attachment", map[string]any{"text": "✅提交成功", "future_attachment": map[string]any{"id": 1}}, false},
		{"album_member", map[string]any{"text": "✅提交成功", "media_group_id": "local-group"}, false},
		{"empty_video_member", map[string]any{"video": map[string]any{"file_id": "local-video"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.message["message_id"] = 1
			tc.message["chat"] = map[string]any{"id": -1004411799859}
			for _, kind := range []string{"channel_post", "edited_channel_post", "message", "edited_message"} {
				raw, err := json.Marshal(map[string]any{"update_id": 1, kind: tc.message})
				if err != nil {
					t.Fatal(err)
				}
				if got := PlainUpdate(raw); got != tc.ignore {
					t.Fatalf("%s ignored=%v want=%v", kind, got, tc.ignore)
				}
			}
		})
	}
	if PlainUpdate([]byte(`{"channel_post":{"text":"✅提交成功"},"message":{"text":"✅提交成功"}}`)) {
		t.Fatal("ambiguous update ignored")
	}
}

func BenchmarkPlainReceipt(b *testing.B) {
	raw := []byte(`{"update_id":1,"channel_post":{"message_id":1,"date":1,"chat":{"id":-1004411799859},"text":"✅ 提交成功"}}`)
	b.ReportAllocs()
	for b.Loop() {
		if !PlainUpdate(raw) {
			b.Fatal("receipt not identified")
		}
	}
}
