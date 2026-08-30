package sys

import (
	"bytes"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var telegramMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

func telegramMarkdownToHTML(markdown string) (string, error) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", nil
	}
	var output bytes.Buffer
	if err := telegramMarkdownRenderer.Convert([]byte(markdown), &output); err != nil {
		return "", gerror.Wrap(err, "解析管理后台 Markdown 失败")
	}
	text := sanitizeTelegramHTML(output.String())
	if strings.TrimSpace(text) == "" {
		return "", gerror.New("管理后台文案不能为空")
	}
	if telegramHTMLTextLength(text) > 4096 {
		return "", gerror.New("管理后台文案不能超过 4096 个字符")
	}
	return text, nil
}
