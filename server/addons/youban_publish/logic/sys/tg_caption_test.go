package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gotd/td/telegram/message/entity"
	gotdhtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestTelegramCaptionMarkHonorsAccountNumberSettings(t *testing.T) {
	row := gdb.Record{
		"title":            gvar.New("资料标题"),
		"profile_no":       gvar.New("A31641"),
		"account_sequence": gvar.New(7),
		"account_nickname": gvar.New("AB"),
	}

	if got := telegramCaptionMark(row, &sysin.AccountSettingModel{EnableTitleMark: 0}); got != "" {
		t.Fatalf("disabled title mark should be empty, got %q", got)
	}
	if got := telegramCaptionMark(row, &sysin.AccountSettingModel{EnableTitleMark: 1, MarkMode: "nickname", NumberSource: "sequence"}); got != "AB007" {
		t.Fatalf("unexpected nickname sequence mark: %q", got)
	}
	if got := telegramCaptionMark(row, &sysin.AccountSettingModel{EnableTitleMark: 1, MarkMode: "custom", CustomMarkText: "xxx", NumberSource: "sequence"}); got != "xxx007" {
		t.Fatalf("unexpected custom sequence mark: %q", got)
	}
	if got := telegramCaptionMark(row, &sysin.AccountSettingModel{EnableTitleMark: 1, MarkMode: "nickname", NumberSource: "random"}); got != "A31641" {
		t.Fatalf("unexpected random mark: %q", got)
	}
	delete(row, "account_sequence")
	if got := telegramCaptionMark(row, &sysin.AccountSettingModel{EnableTitleMark: 1, MarkMode: "nickname", NumberSource: "sequence"}); got != "" {
		t.Fatalf("sequence mark should not fall back to random profile number, got %q", got)
	}
}

func TestTelegramCaptionMarkDoesNotUseProfileTitle(t *testing.T) {
	row := gdb.Record{
		"title":            gvar.New("资料标题"),
		"profile_no":       gvar.New("A31641"),
		"account_sequence": gvar.New(1),
		"account_nickname": gvar.New("AB"),
	}
	setting := &sysin.AccountSettingModel{EnableTitleMark: 1, MarkMode: "nickname", NumberSource: "sequence"}
	if got := buildTelegramTaskCaption(row, setting); got != "AB001" {
		t.Fatalf("caption should contain only the configured mark, got %q", got)
	}
}

func TestTelegramRichTextHTMLKeepsLineBreaks(t *testing.T) {
	input := `<p><strong>天美传媒</strong> 招聊手合作，</p><p>@tmcmkfbot 负责人: @timi_by</p><blockquote><p>⚠️ 新系统，不需要添加机器人</p><p>直接通过机器人自助提交频道链接</p></blockquote><p>🔥外围/中圈/日本女优/韩国明星</p>`
	got := telegramRichTextHTML(input)
	want := "<b>天美传媒</b> 招聊手合作，\n@tmcmkfbot 负责人: @timi_by\n<blockquote>⚠️ 新系统，不需要添加机器人\n直接通过机器人自助提交频道链接</blockquote>\n🔥外围/中圈/日本女优/韩国明星"
	if got != want {
		t.Fatalf("unexpected telegram html:\nwant: %q\n got: %q", want, got)
	}
}

func TestGotdHTMLParserKeepsConvertedLineBreaks(t *testing.T) {
	caption := telegramRichTextHTML(`<p>第一行</p><p>第二行</p>`)
	builder := entity.Builder{}
	if err := styling.Perform(&builder, gotdhtml.String(nil, caption)); err != nil {
		t.Fatal(err)
	}
	text, _ := builder.Complete()
	if text != "第一行\n第二行" {
		t.Fatalf("unexpected gotd text: %q", text)
	}
}

func TestTelegramRichTextHTMLKeepsFormattingAndCustomEmoji(t *testing.T) {
	input := `<tg-emoji emoji-id="5976568857786062743">😂</tg-emoji> <b>史上最强聊手上架管理工具</b>`
	caption := telegramRichTextHTML(input)
	if caption != input {
		t.Fatalf("unexpected telegram html: %q", caption)
	}
	builder := entity.Builder{}
	if err := styling.Perform(&builder, gotdhtml.String(nil, caption)); err != nil {
		t.Fatal(err)
	}
	text, entities := builder.Complete()
	if text != "😂 史上最强聊手上架管理工具" {
		t.Fatalf("unexpected gotd text: %q", text)
	}
	if len(entities) != 2 {
		t.Fatalf("expected custom emoji and bold entities, got %#v", entities)
	}
}

func TestTelegramRichTextHTMLConvertsBreaksAndEmptyParagraphs(t *testing.T) {
	input := `<p>第一行<br>第二行</p><p></p><p>第四行</p>`
	got := telegramRichTextHTML(input)
	want := "第一行\n第二行\n\n第四行"
	if got != want {
		t.Fatalf("unexpected telegram html:\nwant: %q\n got: %q", want, got)
	}
}
