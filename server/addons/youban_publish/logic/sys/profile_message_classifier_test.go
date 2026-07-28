package sys

import "testing"

func TestClassifyProfileMessage(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		media     []collectMediaItem
		wantKind  profileMessageKind
		wantMedia int
	}{
		{
			name:      "text only profile",
			text:      "昵称：A1\n年龄：21",
			wantKind:  profileMessageKindDisplay,
			wantMedia: 0,
		},
		{
			name:      "image and text profile",
			text:      "昵称：A1\n所在城市：北京",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}},
			wantKind:  profileMessageKindDisplay,
			wantMedia: 1,
		},
		{
			name:      "tianmei english fields",
			text:      "罗密欧➕21号\n177D 04\n🍒Age：04年\n🍒Height：177cm\n🍒Weight：56KG",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}},
			wantKind:  profileMessageKindDisplay,
			wantMedia: 1,
		},
		{
			name:      "a1430 chinese fields",
			text:      "🌷编号：N43983\n昵称：霆霆\n省份：江苏\n城市：南京\n年龄：18\n身高：170\n体重：63",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}},
			wantKind:  profileMessageKindDisplay,
			wantMedia: 1,
		},
		{
			name:      "single verify video",
			media:     []collectMediaItem{{Type: "video", FileId: "video-1"}},
			wantKind:  profileMessageKindVerify,
			wantMedia: 1,
		},
		{
			name:      "mixed media without text",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}, {Type: "video", FileId: "video-1"}},
			wantKind:  profileMessageKindIgnore,
			wantMedia: 0,
		},
		{
			name:      "bot notice",
			text:      "✅提交成功！",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}},
			wantKind:  profileMessageKindIgnore,
			wantMedia: 0,
		},
		{
			name:      "number only index message",
			text:      "🌷编号: J16689",
			media:     []collectMediaItem{{Type: "image", FileId: "image-1"}},
			wantKind:  profileMessageKindIgnore,
			wantMedia: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyProfileMessage(test.text, test.media)
			if got.Kind != test.wantKind {
				t.Fatalf("kind = %q, want %q", got.Kind, test.wantKind)
			}
			if len(got.Media) != test.wantMedia {
				t.Fatalf("media count = %d, want %d", len(got.Media), test.wantMedia)
			}
		})
	}
}

func TestProfileMessageHasProfileTextSupportsFieldLabels(t *testing.T) {
	valid := []string{
		"Age：04年\nHeight：177cm\nWeight：56KG",
		"所在省份：甘肃\n所在城市：张掖",
		"nickname: B178\nprovince: 重庆\ncity: 重庆市",
		"Age04年\nHeight173\n体重60kg",
	}
	for _, text := range valid {
		if !profileMessageHasProfileText(text) {
			t.Fatalf("expected profile text: %q", text)
		}
	}
	invalid := []string{"🌷编号: J16689", "随手拍了一张照片", "自拍视频"}
	for _, text := range invalid {
		if profileMessageHasProfileText(text) {
			t.Fatalf("expected non-profile text: %q", text)
		}
	}
}
