package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMessageTemplateUsesInline(t *testing.T) {
	tests := []struct {
		name     string
		template *sysin.MessageTemplateModel
		want     bool
	}{
		{name: "text", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Text: "text"}, want: true},
		{name: "single image", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "image"}}}, want: true},
		{name: "single video", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "video"}}}, want: false},
		{name: "multiple images", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "image"}, {MediaType: "image"}}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageTemplateUsesInline(test.template); got != test.want {
				t.Fatalf("messageTemplateUsesInline() = %v, want %v", got, test.want)
			}
		})
	}
}
