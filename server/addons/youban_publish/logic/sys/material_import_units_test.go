package sys

import "testing"

func TestMaterialImportMergeCrossPageLeadingUnits(t *testing.T) {
	newerPage := []*materialImportMessageUnit{
		{
			GroupedId: "14277572021261444",
			MessageId: 96,
			Messages:  []int{96},
			Media:     []collectMediaItem{{Type: "photo"}},
		},
		{
			MessageId: 97,
			Messages:  []int{97},
			Media:     []collectMediaItem{{Type: "video"}},
		},
	}

	processable, pending := materialImportSplitLeadingUnits(newerPage)
	if len(processable) != 0 {
		t.Fatalf("expected no processable units from newer page, got %d", len(processable))
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending units from newer page, got %d", len(pending))
	}

	olderPage := []*materialImportMessageUnit{
		{
			GroupedId: "14277572021261444",
			RawText:   "昵称：八方来财324",
			MessageId: 93,
			Messages:  []int{93, 94, 95},
			Media: []collectMediaItem{
				{Type: "photo"},
				{Type: "photo"},
				{Type: "photo"},
			},
		},
	}
	processable, carry := materialImportSplitLeadingUnits(olderPage)
	if len(carry) != 0 {
		t.Fatalf("expected no carry units from older page, got %d", len(carry))
	}

	merged := materialImportMergeAdjacentUnits(append(processable, pending...))
	if len(merged) != 2 {
		t.Fatalf("expected display unit and verify unit, got %d", len(merged))
	}
	if got := len(merged[0].Messages); got != 4 {
		t.Fatalf("expected display messages 93-96, got %d", got)
	}
	if merged[0].Messages[0] != 93 || merged[0].Messages[3] != 96 {
		t.Fatalf("unexpected display message ids: %#v", merged[0].Messages)
	}
	if merged[1].MessageId != 97 || len(merged[1].Media) != 1 || merged[1].Media[0].Type != "video" {
		t.Fatalf("unexpected verify unit: %#v", merged[1])
	}
}

func TestMaterialImportTitleFallbackLeadingText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		title     string
		profileNo string
		nickname  string
	}{
		{
			name:      "leading title line",
			text:      "朴朴芙蓉B3054\n省份: 郑州\n城市：开封\n年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "title prefix",
			text:      "标题：朴朴芙蓉B3054\n省份: 郑州\n城市：开封",
			title:     "朴朴芙蓉B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "number and nickname",
			text:      "编号：B3054\n昵称：朴朴芙蓉\n省份: 郑州",
			title:     "B3054",
			profileNo: "B3054",
			nickname:  "朴朴芙蓉",
		},
		{
			name:      "nickname only",
			text:      "昵称：朴朴芙蓉\n省份: 郑州\n城市：开封",
			title:     "朴朴芙蓉",
			profileNo: "",
			nickname:  "朴朴芙蓉",
		},
		{
			name:      "nickname with located province and city",
			text:      "昵称:\u00a0B182\n所在省份:江苏\n所在城市:南京\n年龄:21\n罩杯:C",
			title:     "B182",
			profileNo: "",
			nickname:  "B182",
		},
		{
			name:      "nickname separated by whitespace",
			text:      "昵称 A26\n省份: 山西\n城市: 山西",
			title:     "A26",
			profileNo: "",
			nickname:  "A26",
		},
		{
			name:      "field label contains whitespace",
			text:      "昵称 A26\n省份: 山西\n城 市: 山西",
			title:     "A26",
			profileNo: "",
			nickname:  "A26",
		},
		{
			name:      "leading number before inline nickname",
			text:      "JJ14 昵称: 小安\n省份：浙江",
			title:     "小安",
			profileNo: "",
			nickname:  "小安",
		},
		{
			name:      "mixed same line",
			text:      "朴朴芙蓉B3054 省份: 郑州 城市：开封 年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "title contains field word",
			text:      "开封城市女孩B3054\n年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, profileNo, nickname := materialImportTitle(tc.text)
			if title != tc.title {
				t.Fatalf("unexpected title: %q", title)
			}
			if profileNo != tc.profileNo {
				t.Fatalf("unexpected profileNo: %q", profileNo)
			}
			if nickname != tc.nickname {
				t.Fatalf("unexpected nickname: %q", nickname)
			}
		})
	}
}

func TestMaterialImportIgnoredNotice(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "duplicate notice", text: "❌ 重复投稿\n168 小时内已提交过高度相似资料\n编号：2607212921"},
		{name: "success notice", text: "✅ 提交成功！ 📋 编号：qwby0723613 投稿已自动审核通过并成功分发到 7 个频道。"},
		{name: "failed notice", text: "收录失败！此信息已存在，无需重复收录 编号：65772177"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !materialImportIgnoredNotice(test.text) {
				t.Fatalf("expected notice to be ignored: %q", test.text)
			}
		})
	}
	if materialImportIgnoredNotice("昵称：A26\n省份：山西\n城市：山西") {
		t.Fatal("valid material was incorrectly classified as a notice")
	}
}

func TestMaterialImportProfileText(t *testing.T) {
	valid := []string{
		"昵称：B182\n年龄：21",
		"昵称：B182\n身高：165",
		"昵称：B182\n体重：45kg",
		"昵称：B182\n所在城市：南京",
		"昵称：B182\n城 市：南京",
	}
	for _, text := range valid {
		if !materialImportProfileText(text) {
			t.Fatalf("expected profile text: %q", text)
		}
	}
	invalid := []string{"哇发我", "P31225", "随手拍了一张照片", "自拍视频"}
	for _, text := range invalid {
		if materialImportProfileText(text) {
			t.Fatalf("expected ordinary media text: %q", text)
		}
	}
}

func TestParseMaterialImportChannelReference(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		channel  string
		username string
	}{
		{name: "public channel url", raw: "https://t.me/TestChannel/16313?single", username: "testchannel"},
		{name: "private channel url", raw: "https://t.me/c/123456/16313", channel: "-100123456"},
		{name: "username", raw: "@TestChannel", username: "testchannel"},
		{name: "numeric id", raw: "-100123456", channel: "-100123456"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel, username, err := parseMaterialImportChannelReference(test.raw)
			if err != nil {
				t.Fatalf("parseMaterialImportChannelReference(%q) error: %v", test.raw, err)
			}
			if channel != test.channel || username != test.username {
				t.Fatalf("parseMaterialImportChannelReference(%q) = (%q, %q), want (%q, %q)", test.raw, channel, username, test.channel, test.username)
			}
		})
	}
}
