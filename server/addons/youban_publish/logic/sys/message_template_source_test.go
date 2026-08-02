package sys

import (
	"reflect"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMergeStoredMessageTemplateMediaKeepsSourceAndTelegramMetadata(t *testing.T) {
	input := []*sysin.MessageTemplateMediaInp{{
		Id:          19,
		MediaType:   "image",
		FileUrl:     "/attachment/current.jpg",
		StoragePath: "/attachment/current.jpg",
		SortIndex:   1,
	}}
	stored := []*sysin.MessageTemplateMediaModel{{
		Id:                    19,
		SourceMessageRecordId: 173,
		FileUrl:               "/attachment/old.jpg",
		StoragePath:           "/attachment/old.jpg",
		PosterUrl:             "/attachment/poster.jpg",
		PosterStoragePath:     "/attachment/poster.jpg",
		TgFileId:              "cached-file-id",
		TgThumbFileId:         "thumb-file-id",
		AssetHash:             "asset-hash",
	}}

	mergeStoredMessageTemplateMedia(input, stored)

	if input[0].FileUrl != "/attachment/current.jpg" || input[0].StoragePath != "/attachment/current.jpg" {
		t.Fatalf("visible media fields must keep submitted values: %+v", input[0])
	}
	if input[0].SourceMessageRecordId != 173 || input[0].TgFileId != stored[0].TgFileId || input[0].TgThumbFileId != stored[0].TgThumbFileId || input[0].AssetHash != stored[0].AssetHash {
		t.Fatalf("hidden source metadata was not preserved: %+v", input[0])
	}
}

func TestMessageTemplateSourceRecordIds(t *testing.T) {
	tests := []struct {
		name     string
		template *sysin.MessageTemplateModel
		want     []int64
	}{
		{name: "text", template: &sysin.MessageTemplateModel{SourceMessageRecordId: 173}, want: []int64{173}},
		{name: "album", template: &sysin.MessageTemplateModel{Media: []*sysin.MessageTemplateMediaModel{{SourceMessageRecordId: 173}, {SourceMessageRecordId: 174}}}, want: []int64{173, 174}},
		{name: "incomplete album", template: &sysin.MessageTemplateModel{Media: []*sysin.MessageTemplateMediaModel{{SourceMessageRecordId: 173}, {}}}, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageTemplateSourceRecordIds(test.template); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("unexpected source ids: got=%v want=%v", got, test.want)
			}
		})
	}
}

func TestMessageTemplateRequiresSourcePreservation(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "custom emoji", text: `<tg-emoji emoji-id="5976568857786062743">🔥</tg-emoji>`, want: true},
		{name: "uppercase tag", text: `<TG-EMOJI EMOJI-ID="5976568857786062743">🔥</TG-EMOJI>`, want: true},
		{name: "regular emoji", text: `🔥 普通Emoji`, want: false},
		{name: "formatted text", text: `<b>粗体</b>`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageTemplateRequiresSourcePreservation(&sysin.MessageTemplateModel{Text: test.text}); got != test.want {
				t.Fatalf("unexpected preservation decision: got=%v want=%v", got, test.want)
			}
		})
	}
}

func TestMessageTemplateSourceContentUnchanged(t *testing.T) {
	storedTemplate := &sysin.MessageTemplateModel{Text: "<b>原文</b>", SourceMessageRecordId: 173}
	storedMedia := []*sysin.MessageTemplateMediaModel{{
		Id:                    20,
		SourceMessageRecordId: 173,
		MediaType:             "image",
		FileUrl:               "/attachment/source.jpg",
		StoragePath:           "attachment/source.jpg",
		SortIndex:             1,
	}}
	input := &sysin.MessageTemplateSaveInp{Text: "<b>原文</b>", Media: []*sysin.MessageTemplateMediaInp{{
		Id:                    20,
		SourceMessageRecordId: 173,
		MediaType:             "image",
		FileUrl:               "/attachment/source.jpg",
		StoragePath:           "attachment/source.jpg",
		SortIndex:             1,
	}}}
	if !messageTemplateSourceContentUnchanged(storedTemplate, storedMedia, input) {
		t.Fatal("unchanged template should keep the source message reference")
	}

	input.Text = "<b>已修改</b>"
	if messageTemplateSourceContentUnchanged(storedTemplate, storedMedia, input) {
		t.Fatal("edited template must detach from the source message")
	}
}
