package sys

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestBuildCollectRuleDecisionAppendsFooterWithoutBlankLine(t *testing.T) {
	event := gdb.Record{
		"raw_text":    gvar.New("今日资料\n编号：R25318"),
		"media_count": gvar.New(1),
	}
	rule := gdb.Record{
		"footer_enabled":  gvar.New(1),
		"footer_markdown": gvar.New("联系客服：@xiaohuiji"),
	}

	decision := buildCollectRuleDecision(event, nil, rule)
	if got, want := decision.Text, "今日资料\n编号：R25318\n联系客服：@xiaohuiji"; got != want {
		t.Fatalf("decision text = %q, want %q", got, want)
	}
}

func TestCollectRuleOnlineCaseMatrix(t *testing.T) {
	base := func() gdb.Record {
		return gdb.Record{
			"block_plain_text": gvar.New(1), "block_link": gvar.New(1), "block_username": gvar.New(1),
			"full_match_enabled": gvar.New(0), "keywords": gvar.New([]string{}), "tags": gvar.New([]string{}),
			"blocked_texts": gvar.New([]string{}), "delete_lines": gvar.New([]string{}), "delete_texts": gvar.New([]string{}),
			"replace_from": gvar.New([]string{}), "replace_to": gvar.New([]string{}), "truncate_intro_fee_enabled": gvar.New(false),
		}
	}
	cases := []struct {
		name, text, want string
		media            int
		mutate           func(gdb.Record)
		matched          bool
	}{
		{name: "normal enters and replaces", text: "正文 A", media: 1, want: "正文 B", mutate: func(r gdb.Record) {
			r["replace_from"] = gvar.New([]string{"A"})
			r["replace_to"] = gvar.New([]string{"B"})
		}, matched: true},
		{name: "no media skipped", text: "正文", media: 0, want: "", mutate: func(gdb.Record) {}, matched: false},
		{name: "blocked text skipped", text: "正文黑名单", media: 1, want: "", mutate: func(r gdb.Record) { r["blocked_texts"] = gvar.New([]string{"黑名单"}) }, matched: false},
		{name: "intro fee truncated", text: "正文\n介绍费：7888\n尾部", media: 1, want: "正文", mutate: func(r gdb.Record) { r["truncate_intro_fee_enabled"] = gvar.New(true) }, matched: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := base()
			tc.mutate(rule)
			event := gdb.Record{"raw_text": gvar.New(tc.text), "media_count": gvar.New(tc.media)}
			pre := precheckCollectRule(event, rule)
			if pre.Matched != tc.matched {
				t.Fatalf("matched=%t want %t reason=%s", pre.Matched, tc.matched, pre.Reason)
			}
			if !tc.matched {
				return
			}
			decision := buildCollectRuleDecision(event, nil, rule)
			if decision.Text != tc.want {
				t.Fatalf("text=%q want %q", decision.Text, tc.want)
			}
		})
	}
}

func collectDedupeMaterialFromValues(textHash string, mediaJSON string) collectDedupeMaterial {
	items := make([]collectMediaItem, 0)
	_ = json.Unmarshal([]byte(mediaJSON), &items)
	return collectDedupeMaterialFromItems(textHash, items)
}

func TestCollectMediaFingerprintSetKeyIsOrderIndependent(t *testing.T) {
	left := []collectMediaItem{
		{Type: "photo", FileId: "photo-a"},
		{Type: "photo", FileId: "photo-b"},
		{Type: "photo", FileId: "photo-c"},
	}
	right := []collectMediaItem{
		{Type: "photo", FileId: "photo-c"},
		{Type: "photo", FileId: "photo-a"},
		{Type: "photo", FileId: "photo-b"},
	}
	if got, want := collectMediaFingerprintSetKey(left), collectMediaFingerprintSetKey(right); got != want {
		t.Fatalf("media fingerprint set key differs by order: %q != %q", got, want)
	}
}

func TestCollectImagePHashSetIgnoresOrderAndVideos(t *testing.T) {
	left := []collectMediaItem{
		{Type: "photo", FilePhash: "A"},
		{Type: "photo", FilePhash: "B"},
		{Type: "video", FilePhash: "video"},
	}
	right := []collectMediaItem{
		{Type: "photo", FilePhash: "b"},
		{Type: "photo", FilePhash: "a"},
	}
	leftKey, leftCount := collectImagePHashSetKey(left)
	rightKey, rightCount := collectImagePHashSetKey(right)
	if leftKey != rightKey || leftCount != 2 || rightCount != 2 {
		t.Fatalf("image phash set mismatch: left=(%q,%d) right=(%q,%d)", leftKey, leftCount, rightKey, rightCount)
	}
}

func TestCollectDedupeSignaturesRequireCompleteMediaMetadata(t *testing.T) {
	partialFingerprint := collectDedupeMaterialFromItems("", []collectMediaItem{
		{Type: "photo", FileId: "photo-1", FilePhash: "hash-1"},
		{Type: "photo"},
	})
	for _, signature := range partialFingerprint.signatures(true) {
		if signature.layer == "media_fingerprint" || signature.layer == "image_phash" {
			t.Fatalf("partial metadata must not produce %s signature", signature.layer)
		}
	}

	complete := collectDedupeMaterialFromItems("text-hash", []collectMediaItem{
		{Type: "photo", FileId: "photo-1", FilePhash: "hash-1"},
		{Type: "photo", FileId: "photo-2", FilePhash: "hash-2"},
	})
	layers := map[string]bool{}
	for _, signature := range complete.signatures(true) {
		layers[signature.layer] = true
	}
	for _, layer := range []string{"text_hash", "media_fingerprint", "image_phash"} {
		if !layers[layer] {
			t.Fatalf("complete material missing %s signature", layer)
		}
	}
}

func TestCollectDedupeLocksOverlapOnAnyMatchingLayer(t *testing.T) {
	left := collectDedupeMaterial{textHash: "same-text", mediaKey: "left-media", mediaTotal: 1, mediaCount: 1}
	right := collectDedupeMaterial{textHash: "same-text", mediaKey: "right-media", mediaTotal: 1, mediaCount: 1}
	leftKeys := collectDedupeSignatureLockKeys(left)
	rightKeys := collectDedupeSignatureLockKeys(right)
	overlaps := false
	for _, leftKey := range leftKeys {
		for _, rightKey := range rightKeys {
			if leftKey == rightKey {
				overlaps = true
			}
		}
	}
	if !overlaps {
		t.Fatal("materials matching any dedupe layer must share a lock")
	}
}

func TestCollectMediaPHashReadsMetadata(t *testing.T) {
	item := collectMediaItem{
		Type:      "photo",
		FilePhash: "ABC123",
	}
	if got := collectMediaPHash(item); got != "abc123" {
		t.Fatalf("media phash = %q, want %q", got, "abc123")
	}
}

func TestCollectDedupeCacheValue(t *testing.T) {
	wantTime := time.Unix(1_785_000_000, 0)
	value := collectDedupeCacheValue(123, wantTime)
	entry, ok := parseCollectDedupeCacheValue(value)
	if !ok || entry.EventID != 123 || entry.ReceivedAt != wantTime.Unix() {
		t.Fatalf("entry = %+v ok=%v", entry, ok)
	}
	if parseEntry, parseOK := parseCollectDedupeCacheValue("invalid"); parseOK || parseEntry.EventID != 0 {
		t.Fatalf("invalid cache value must be rejected: %+v %v", parseEntry, parseOK)
	}
}

func TestCollectDedupeCacheEntryValid(t *testing.T) {
	now := time.Unix(1_785_000_000, 0)
	recent := collectDedupeCacheEntry{EventID: 1, ReceivedAt: now.AddDate(0, 0, -2).Unix()}
	if !collectDedupeCacheEntryValid(recent, 3, now) {
		t.Fatal("recent entry must be valid inside the time window")
	}
	if collectDedupeCacheEntryValid(recent, 1, now) {
		t.Fatal("old entry must be invalid outside the time window")
	}
	if !collectDedupeCacheEntryValid(recent, 0, now) {
		t.Fatal("zero-day window must have no expiration")
	}
}

func TestApplyCollectIntroFeeTruncate(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "removes matched line and following text", text: "标题\n介绍费 7888\nKK", want: "标题"},
		{name: "removes all text when first line matches", text: "介绍费 7888\nKK", want: ""},
		{name: "supports windows line endings", text: "标题\r\n介绍费：7888\r\nKK", want: "标题"},
		{name: "matches intro fee with invisible format characters", text: "标题\n介\u200b绍\u200d费：7888(香水湾💦)⁣‌\n联系方式", want: "标题"},
		{name: "matches recommendation fee", text: "标题\n推荐费：7888 情诗X\n介绍费:7889\u2060⁣‌", want: "标题"},
		{name: "matches referral fee synonyms", text: "标题\n中介费：7888\n尾部", want: "标题"},
		{name: "matches brokerage fee synonym", text: "标题\n牵线费用：7888\n尾部", want: "标题"},
		{name: "matches intermediary fee synonym", text: "标题\n居间费：7888\n尾部", want: "标题"},
		{name: "matches connection fee synonym", text: "标题\n对接费用：6888\n尾部", want: "标题"},
		{name: "matches brokerage service fee", text: "标题\n中介服务费7888\n尾部", want: "标题"},
		{name: "matches variation selectors inside keyword", text: "标题\n介︇绍️费：7888\n尾部", want: "标题"},
		{name: "cleans joined suffix with invisible tail", text: "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费7888TT​‌⁣‌​​​‌​‌​​‌‌‌‌​‌‌‌‌​​‌‌‌​‌‌‌​‌​‌‌‌‌‌‌​​‌​​​​‌‌​‌‌​​​​​‌‌‌​‌​‌​​​‌‌​‌‌‌​‌‌​‌⁤", want: "雷点（不能接受的）：拍照 户外 手指 肛交 多"},
		{name: "removes source mark before intro fee", text: "个人优点:反差、私生活少、嫩\n不能接受金主:身高170以下、太胖\n情诗w41\n七七xq\n介绍费:7889", want: "个人优点:反差、私生活少、嫩\n不能接受金主:身高170以下、太胖\n七七xq"},
		{name: "removes obfuscated source mark line", text: "正文\n情\u2060诗 W41\n介绍费:7889", want: "正文"},
		{name: "keeps text without keyword", text: "标题\n联系方式\nKK", want: "标题\n联系方式\nKK"},
		{name: "removes leading profile metadata", text: "昵称：朴朴\n编号：XXX123\n同行：否\n正常文案", want: "正常文案"},
		{name: "removes leading latin and chinese codes", text: "XXX123\n朴朴123123\n正常文案", want: "正常文案"},
		{name: "removes any non-chinese first line", text: "ABC-20260811\n正常文案", want: "正常文案"},
		{name: "removes english first line", text: "English marker\n正常文案", want: "正常文案"},
		{name: "removes consecutive non-chinese header lines", text: "A1\nABCDEFG123456789\n---\n正常文案", want: "正常文案"},
		{name: "removes non-chinese header after blank line", text: "\nA1\nB20260811\n正常文案", want: "正常文案"},
		{name: "removes metadata then consecutive codes", text: "昵称：朴朴\nA1\nB20260811\n正常文案", want: "正常文案"},
		{name: "keeps chinese first line", text: "English中文 marker\n正常文案", want: "English中文 marker\n正常文案"},
		{name: "removes metadata before intro fee and following text", text: "昵称：朴朴\nX123\n正常文案\n介绍费：7888\n联系方式", want: "正常文案"},
		{name: "recognizes fullwidth semicolon fee separator", text: "介绍人；柏林之声    介绍费；7888\n七七b\n介绍费:7888", want: ""},
		{name: "removes metadata fields inside body", text: "正常文案\n昵称：朴朴\n联系方式\n编号：XXX123\n同行：否", want: "正常文案\n联系方式"},
		{name: "keeps metadata words in normal body", text: "这是昵称说明\n编号是内部记录\n同行可以联系", want: "这是昵称说明\n编号是内部记录\n同行可以联系"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applyCollectIntroFeeTruncate(test.text); got != test.want {
				t.Fatalf("truncate text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyCollectIntroFeeSuffix(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		original string
		suffix   string
		want     string
	}{
		{name: "space suffix", text: "正文\n介绍费 7888 KK", original: "正文\n介绍费 7888 KK", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "joined suffix", text: "正文\n介绍费 7888KK", original: "正文\n介绍费 7888KK", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "joined suffix with invisible tail", text: "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费7888TT​‌⁣‌​​​‌​‌​​‌‌‌‌​‌‌‌‌​​‌‌‌​‌‌‌​‌​‌‌‌‌‌‌​​‌​​​​‌‌​‌‌​​​​​‌‌‌​‌​‌​​​‌‌​‌‌‌​‌‌​‌⁤", original: "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费7888TT​‌⁣‌​​​‌​‌​​‌‌‌‌​‌‌‌‌​​‌‌‌​‌‌‌​‌​‌‌‌‌‌‌​​‌​​​​‌‌​‌‌​​​​​‌‌‌​‌​‌​​​‌‌​‌‌‌​‌‌​‌⁤", suffix: "AA", want: "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费 7888 AA"},
		{name: "joined chinese suffix", text: "正文\n介绍费:7888七七n", original: "正文\n介绍费:7888七七n", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "hyphen suffix", text: "正文\n介绍费：6888 BLY-54", original: "正文\n介绍费：6888 BLY-54", suffix: "AA", want: "正文\n介绍费 6888 AA"},
		{name: "colon suffix", text: "正文\n介绍费: 7888 KK", original: "正文\n介绍费: 7888 KK", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "suffix on next line", text: "正文\n介绍费 7888\nKK", original: "正文\n介绍费 7888\nKK", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "long suffix on next line", text: "正文\n介绍费7888\nBLYS", original: "正文\n介绍费7888\nBLYS", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "preserves following content", text: "正文\n介绍费 7888 KK\n联系方式", original: "正文\n介绍费 7888 KK\n联系方式", suffix: "AA", want: "正文\n联系方式\n介绍费 7888 AA"},
		{name: "amount punctuation", text: "正文\n介绍费用：7,888.50 元 KK", original: "正文\n介绍费用：7,888.50 元 KK", suffix: "AA", want: "正文\n介绍费 7,888.50 元 AA"},
		{name: "recommendation fee removes duplicated intro fee", text: "推荐费：7888 情诗X\n七七xq\n介绍费:7889\u2060⁣‌", original: "推荐费：7888 情诗X\n七七xq\n介绍费:7889\u2060⁣‌", suffix: "七七xq", want: "介绍费 7888 七七xq"},
		{name: "obfuscated brokerage fee", text: "正文\n牵︊线️费：7888 燕姐", original: "正文\n牵︊线️费：7888 燕姐", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "windows lines", text: "正文\r\n介绍费：7888\r\nKK", original: "正文\r\n介绍费：7888\r\nKK", suffix: "AA", want: "正文\n介绍费 7888 AA"},
		{name: "no fee", text: "正文\n联系方式", original: "正文\n联系方式", suffix: "AA", want: "正文\n联系方式"},
		{name: "disabled", text: "正文\n介绍费 7888 KK", original: "正文\n介绍费 7888 KK", suffix: "", want: "正文\n介绍费 7888 KK"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applyCollectIntroFeeSuffix(test.text, test.original, test.suffix); got != test.want {
				t.Fatalf("applyCollectIntroFeeSuffix() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsCollectIntroFeeStandaloneSuffix(t *testing.T) {
	for _, text := range []string{"KK", "BLYS", "七七xq", "A5"} {
		if !isCollectIntroFeeStandaloneSuffix(text) {
			t.Fatalf("%q should be recognized as fee suffix", text)
		}
	}
	for _, text := range []string{"联系方式", "联系七七", "正文内容"} {
		if isCollectIntroFeeStandaloneSuffix(text) {
			t.Fatalf("%q should be preserved as content", text)
		}
	}
}

func TestBuildCollectRuleDecisionRestoresIntroFeeAfterTruncate(t *testing.T) {
	event := gdb.Record{"raw_text": gvar.New("编号：A123\n正文\n介绍费 7888\nKK"), "media_count": gvar.New(1)}
	rule := gdb.Record{
		"truncate_intro_fee_enabled": gvar.New(true), "intro_fee_suffix": gvar.New("AA"),
		"delete_lines": gvar.New([]string{}), "delete_texts": gvar.New([]string{}),
		"replace_from": gvar.New([]string{}), "replace_to": gvar.New([]string{}),
	}
	if got, want := buildCollectRuleDecision(event, nil, rule).Text, "正文\n介绍费 7888 AA"; got != want {
		t.Fatalf("decision text = %q, want %q", got, want)
	}
}

func TestBuildCollectRuleDecisionRemovesOriginalSourceMarkAndPreservesFooter(t *testing.T) {
	event := gdb.Record{
		"raw_text":    gvar.New("个人优点:反差、私生活少、嫩\n情诗w41\n介绍费:7888 情诗X"),
		"media_count": gvar.New(2),
	}
	rule := gdb.Record{
		"truncate_intro_fee_enabled": gvar.New(true),
		"footer_enabled":             gvar.New(1),
		"footer_markdown":            gvar.New("七七xq\n介绍费:7889"),
		"delete_lines":               gvar.New([]string{}),
		"delete_texts":               gvar.New([]string{}),
		"replace_from":               gvar.New([]string{}),
		"replace_to":                 gvar.New([]string{}),
	}
	want := "个人优点:反差、私生活少、嫩\n七七xq\n介绍费:7889"
	if got := buildCollectRuleDecision(event, nil, rule).Text; got != want {
		t.Fatalf("decision text = %q, want %q", got, want)
	}
}

func TestBuildCollectRuleDecisionReplacesJoinedIntroFeeSuffix(t *testing.T) {
	raw := "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费7888TT​‌⁣‌​​​‌​‌​​‌‌‌‌​‌‌‌‌​​‌‌‌​‌‌‌​‌​‌‌‌‌‌‌​​‌​​​​‌‌​‌‌​​​​​‌‌‌​‌​‌​​​‌‌​‌‌‌​‌‌​‌⁤"
	event := gdb.Record{"raw_text": gvar.New(raw), "media_count": gvar.New(1)}
	rule := gdb.Record{
		"truncate_intro_fee_enabled": gvar.New(true), "intro_fee_suffix": gvar.New("AA"),
		"delete_lines": gvar.New([]string{}), "delete_texts": gvar.New([]string{}),
		"replace_from": gvar.New([]string{}), "replace_to": gvar.New([]string{}),
	}
	if got, want := buildCollectRuleDecision(event, nil, rule).Text, "雷点（不能接受的）：拍照 户外 手指 肛交 多\n介绍费 7888 AA"; got != want {
		t.Fatalf("decision text = %q, want %q", got, want)
	}
}

func TestNormalizeCollectMaterialTextPreservesModifiedIntroFee(t *testing.T) {
	s := &sSysPublish{}
	event := gdb.Record{"id": gvar.New(int64(105)), "raw_text": gvar.New("正文\n介绍费 7888 KK")}
	rule := gdb.Record{
		"id": gvar.New(int64(50)), "truncate_intro_fee_enabled": gvar.New(true),
		"intro_fee_suffix": gvar.New("AA"),
	}
	if got, want := s.normalizeCollectMaterialText(context.Background(), event, rule, "正文\n介绍费 7888 AA"), "正文\n介绍费 7888 AA"; got != want {
		t.Fatalf("commit boundary text = %q, want %q", got, want)
	}
}

func TestBuildCollectRuleDecisionKeepsIntroFeeTruncatedAcrossRepeatedProcessing(t *testing.T) {
	rawText := "省份：广东\n城市：广州\n介绍费：7888(香水湾💦)"
	event := gdb.Record{
		"raw_text":    gvar.New(rawText),
		"media_count": gvar.New(2),
	}
	rule := gdb.Record{
		"truncate_intro_fee_enabled": gvar.New(true),
		"delete_lines":               gvar.New([]string{}),
		"delete_texts":               gvar.New([]string{}),
		"replace_from":               gvar.New([]string{}),
		"replace_to":                 gvar.New([]string{}),
		"blocked_texts":              gvar.New([]string{}),
	}

	first := buildCollectRuleDecision(event, nil, rule)
	second := buildCollectRuleDecision(event, nil, rule)
	if first.Text != "省份：广东\n城市：广州" || second.Text != first.Text {
		t.Fatalf("repeated rule processing produced inconsistent text: first=%q second=%q", first.Text, second.Text)
	}
	if event["raw_text"].String() != rawText {
		t.Fatalf("rule processing mutated raw text: %q", event["raw_text"].String())
	}
}

func TestNormalizeCollectMaterialTextRemovesIntroFeeAtCommitBoundary(t *testing.T) {
	s := &sSysPublish{}
	event := gdb.Record{"id": gvar.New(int64(101))}
	rule := gdb.Record{"id": gvar.New(int64(46)), "truncate_intro_fee_enabled": gvar.New(true)}
	text := "正文\n介绍费：7888(香水湾💦)"
	if got, want := s.normalizeCollectMaterialText(context.Background(), event, rule, text), "正文"; got != want {
		t.Fatalf("commit boundary text = %q, want %q", got, want)
	}
}

func TestNormalizeCollectMaterialTextPreservesOriginalWhenEditingDisabled(t *testing.T) {
	s := &sSysPublish{}
	event := gdb.Record{"id": gvar.New(int64(102))}
	rule := gdb.Record{"id": gvar.New(int64(47)), "truncate_intro_fee_enabled": gvar.New(false)}
	text := "正文\n介绍费：7888(香水湾💦)"
	if got := s.normalizeCollectMaterialText(context.Background(), event, rule, text); got != text {
		t.Fatalf("disabled edit changed original text: got %q want %q", got, text)
	}
}

func TestNormalizeCollectMaterialTextPreservesEmojiAndLineBreaks(t *testing.T) {
	s := &sSysPublish{}
	event := gdb.Record{"id": gvar.New(int64(103))}
	rule := gdb.Record{"id": gvar.New(int64(48)), "truncate_intro_fee_enabled": gvar.New(true)}
	text := "标题💦\n正文第一行\n正文第二行\n介绍费：7888(香水湾💦)"
	if got, want := s.normalizeCollectMaterialText(context.Background(), event, rule, text), "标题💦\n正文第一行\n正文第二行"; got != want {
		t.Fatalf("cleaned text lost formatting: got %q want %q", got, want)
	}
}

func TestNormalizeCollectMaterialTextPreservesFooterIntroFee(t *testing.T) {
	s := &sSysPublish{}
	event := gdb.Record{"id": gvar.New(int64(104))}
	rule := gdb.Record{
		"id":                         gvar.New(int64(49)),
		"truncate_intro_fee_enabled": gvar.New(true),
		"footer_markdown":            gvar.New("追加文案：介绍费请联系客服"),
	}
	text := "正文\n介绍费：7888\n追加文案：介绍费请联系客服"
	want := text
	if got := s.normalizeCollectMaterialText(context.Background(), event, rule, text); got != want {
		t.Fatalf("footer intro fee was removed: got %q want %q", got, want)
	}
}
