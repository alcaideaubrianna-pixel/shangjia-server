package sys

import (
	"testing"

	"github.com/gotd/td/telegram/message/entity"
	gotdhtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
)

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

func TestTelegramRichTextHTMLConvertsBreaksAndEmptyParagraphs(t *testing.T) {
	input := `<p>第一行<br>第二行</p><p></p><p>第四行</p>`
	got := telegramRichTextHTML(input)
	want := "第一行\n第二行\n\n第四行"
	if got != want {
		t.Fatalf("unexpected telegram html:\nwant: %q\n got: %q", want, got)
	}
}
