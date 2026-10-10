package sys

import (
	"strings"
	"testing"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfilePreviewDisplayCaptionIncludesCollectionSource(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
		ProfileNo:             "V76621",
		Title:                 "海南 琼海",
		PlainText:             "年龄：17\n身高：160",
		CollectSourceName:     "春风庭 @cftbyong1",
		CollectSourceUsername: "cftbyong1",
		CollectSourceUrl:      "https://t.me/cftbyong1/1674",
	}}

	caption := profilePreviewDisplayCaption(note)
	for _, expected := range []string{
		"资料来源：春风庭 @cftbyong1",
		`资料链接：<a href="https://t.me/cftbyong1/1674">t.me/cftbyong1/1674</a>`,
		"年龄：17",
	} {
		if !strings.Contains(caption, expected) {
			t.Fatalf("caption %q does not contain %q", caption, expected)
		}
	}
}

func TestProfileSourceDisplayUsesSourceMetadataWithoutCollectedFlag(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
		CollectSourceUsername: "cftbyong1",
		CollectSourceUrl:      "https://t.me/cftbyong1/1674",
		IsCollected:           false,
	}}

	display := profileSourceDisplay(note)
	if !strings.Contains(display, "资料来源：cftbyong1") || !strings.Contains(display, "t.me/cftbyong1/1674") {
		t.Fatalf("unexpected source display: %q", display)
	}
}

func TestProfileSourceDisplayFallsBackForURLOnlyMetadata(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
		CollectSourceUrl: "https://t.me/cftbyong1/1674",
	}}

	display := profileSourceDisplay(note)
	if !strings.Contains(display, "资料来源：来源频道") || !strings.Contains(display, "t.me/cftbyong1/1674") {
		t.Fatalf("unexpected source display: %q", display)
	}
}
