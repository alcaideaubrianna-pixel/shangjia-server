package sys

import (
	"context"
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	xhtml "golang.org/x/net/html"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) telegramJobCaption(ctx context.Context, job telegramJobRecord) (string, error) {
	row, err := s.profilePublishSource(ctx, job.ProfileId, job.TenantId, job.AccountId, false)
	if err != nil {
		return "", err
	}
	setting, err := s.accountSetting(ctx, row["tenant_id"].Int64(), row["account_id"].Int64())
	if err != nil {
		return "", err
	}
	caption := buildTelegramTaskCaption(row, setting)
	if strings.TrimSpace(caption) == "" {
		g.Log().Warningf(ctx, "TG推送文案为空 profileId:%d sourceType:%s operationNo:%s plainTextBytes:%d summaryBytes:%d titleBytes:%d", job.ProfileId, row["source_type"].String(), job.OperationNo, len(strings.TrimSpace(row["plain_text"].String())), len(strings.TrimSpace(row["summary"].String())), len(strings.TrimSpace(row["title"].String())))
	}
	return caption, nil
}

func buildTelegramTaskCaption(row gdb.Record, setting *sysin.AccountSettingModel) string {
	if isCollectPublishTask(row) {
		return telegramEscapeText(profileTelegramText(row))
	}
	lines := make([]string, 0, 6)
	mark := telegramCaptionMark(row, setting)
	if setting != nil && setting.EnableTitleMark == 1 && setting.MarkPosition == "top" && mark != "" {
		lines = append(lines, telegramEscapeText(mark))
	}
	if text := strings.TrimSpace(row["plain_text"].String()); text != "" {
		lines = append(lines, telegramEscapeText(text))
	}
	if setting != nil && setting.EnableTitleMark == 1 && setting.MarkPosition != "top" && mark != "" {
		lines = appendCaptionMark(lines, telegramEscapeText(mark), setting.MarkPosition)
	}
	if setting != nil && setting.EnableSuffix == 1 {
		if suffix := telegramRichTextHTML(setting.SuffixContent); suffix != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, suffix)
		}
	}
	return strings.Join(lines, "\n")
}

func profileTelegramText(row gdb.Record) string {
	for _, key := range []string{"plain_text", "summary", "title"} {
		if text := strings.TrimSpace(row[key].String()); text != "" {
			return text
		}
	}
	return ""
}

func isCollectPublishTask(row gdb.Record) bool {
	return strings.HasPrefix(strings.TrimSpace(row["client_request_id"].String()), "collect:")
}

func appendCaptionMark(lines []string, mark string, position string) []string {
	if position == "feeLine" {
		pattern := regexp.MustCompile(`(?i)(介绍费|介绍费用|服务费|费用)`)
		for index, line := range lines {
			if pattern.MatchString(line) {
				lines[index] = strings.TrimRight(line, " \t") + " " + mark
				return lines
			}
		}
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, mark)
}

func telegramCaptionMark(row gdb.Record, setting *sysin.AccountSettingModel) string {
	if setting == nil || setting.EnableTitleMark != 1 {
		return ""
	}
	number := telegramCaptionNumber(row, setting.NumberSource)
	if number == "" {
		return ""
	}
	if setting.NumberSource == "random" {
		return number
	}
	prefix := markPrefix(setting, row["account_nickname"].String())
	if prefix == "" {
		return number
	}
	return fmt.Sprintf("%s%s", prefix, number)
}

func telegramCaptionNumber(row gdb.Record, numberSource string) string {
	if numberSource == "random" {
		return strings.TrimSpace(row["profile_no"].String())
	}
	if sequence := row["account_sequence"].Int64(); sequence > 0 {
		return fmt.Sprintf("%03d", sequence)
	}
	return ""
}

func telegramEscapeText(value string) string {
	return stdhtml.EscapeString(strings.TrimSpace(value))
}

func telegramSuffixHTML(value string) string {
	return telegramRichTextHTML(value)
}

func telegramRichTextHTML(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(value), nil)
	if err != nil {
		return telegramEscapeText(value)
	}
	var builder strings.Builder
	for _, node := range nodes {
		writeTelegramHTMLNode(&builder, node)
	}
	return strings.TrimSpace(builder.String())
}

// TelegramRichTextHTML exposes the canonical rich-text conversion to other
// addons that produce Telegram messages.
func (s *sSysPublish) TelegramRichTextHTML(value string) string {
	return telegramRichTextHTML(value)
}

func writeTelegramHTMLNode(builder *strings.Builder, node *xhtml.Node) {
	if node == nil {
		return
	}
	switch node.Type {
	case xhtml.TextNode:
		builder.WriteString(stdhtml.EscapeString(node.Data))
	case xhtml.ElementNode:
		writeTelegramHTMLElement(builder, node)
	default:
		writeTelegramHTMLChildren(builder, node)
	}
}

func writeTelegramHTMLElement(builder *strings.Builder, node *xhtml.Node) {
	switch strings.ToLower(node.Data) {
	case "b", "strong":
		writeTelegramWrappedElement(builder, "b", node)
	case "i", "em":
		writeTelegramWrappedElement(builder, "i", node)
	case "u", "ins":
		writeTelegramWrappedElement(builder, "u", node)
	case "s", "strike", "del":
		writeTelegramWrappedElement(builder, "s", node)
	case "code":
		writeTelegramWrappedElement(builder, "code", node)
	case "pre":
		writeTelegramWrappedElement(builder, "pre", node)
	case "blockquote":
		writeTelegramBlockquoteElement(builder, node)
	case "tg-spoiler":
		writeTelegramWrappedElement(builder, "tg-spoiler", node)
	case "tg-emoji":
		writeTelegramEmojiElement(builder, node)
	case "tg-time":
		writeTelegramTimeElement(builder, node)
	case "span":
		writeTelegramSpanElement(builder, node)
	case "a":
		writeTelegramLinkElement(builder, node)
	case "br":
		builder.WriteString("\n")
	case "p", "div":
		writeTelegramParagraphElement(builder, node)
	case "ul", "ol":
		writeTelegramListElement(builder, node)
	case "li":
		writeTelegramListItemElement(builder, node)
	default:
		writeTelegramHTMLChildren(builder, node)
	}
}

func writeTelegramSpanElement(builder *strings.Builder, node *xhtml.Node) {
	if telegramAttr(node, "data-telegram-spoiler") == "true" || strings.Contains(" "+telegramAttr(node, "class")+" ", " tg-spoiler ") {
		writeTelegramWrappedElement(builder, "tg-spoiler", node)
		return
	}
	writeTelegramHTMLChildren(builder, node)
}

func writeTelegramBlockquoteElement(builder *strings.Builder, node *xhtml.Node) {
	if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("<blockquote")
	if _, ok := telegramAttrValue(node, "expandable"); ok {
		builder.WriteString(" expandable")
	}
	builder.WriteString(">")
	var children strings.Builder
	writeTelegramHTMLChildren(&children, node)
	builder.WriteString(strings.Trim(children.String(), "\n"))
	builder.WriteString("</blockquote>")
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
}

func writeTelegramEmojiElement(builder *strings.Builder, node *xhtml.Node) {
	emojiId := telegramAttr(node, "emoji-id")
	if emojiId == "" {
		writeTelegramHTMLChildren(builder, node)
		return
	}
	builder.WriteString(`<tg-emoji emoji-id="`)
	builder.WriteString(stdhtml.EscapeString(emojiId))
	builder.WriteString(`">`)
	writeTelegramHTMLChildren(builder, node)
	builder.WriteString("</tg-emoji>")
}

func writeTelegramTimeElement(builder *strings.Builder, node *xhtml.Node) {
	unix := telegramAttr(node, "unix")
	if unix == "" {
		writeTelegramHTMLChildren(builder, node)
		return
	}
	builder.WriteString(`<tg-time unix="`)
	builder.WriteString(stdhtml.EscapeString(unix))
	builder.WriteString(`"`)
	if format := telegramAttr(node, "format"); format != "" {
		builder.WriteString(` format="`)
		builder.WriteString(stdhtml.EscapeString(format))
		builder.WriteString(`"`)
	}
	builder.WriteString(">")
	writeTelegramHTMLChildren(builder, node)
	builder.WriteString("</tg-time>")
}

func writeTelegramWrappedElement(builder *strings.Builder, tag string, node *xhtml.Node) {
	builder.WriteString("<")
	builder.WriteString(tag)
	builder.WriteString(">")
	writeTelegramHTMLChildren(builder, node)
	builder.WriteString("</")
	builder.WriteString(tag)
	builder.WriteString(">")
}

func writeTelegramLinkElement(builder *strings.Builder, node *xhtml.Node) {
	href := telegramAttr(node, "href")
	if href == "" {
		writeTelegramHTMLChildren(builder, node)
		return
	}
	builder.WriteString(`<a href="`)
	builder.WriteString(stdhtml.EscapeString(href))
	builder.WriteString(`">`)
	writeTelegramHTMLChildren(builder, node)
	builder.WriteString("</a>")
}

func writeTelegramParagraphElement(builder *strings.Builder, node *xhtml.Node) {
	if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
	beforeLen := builder.Len()
	writeTelegramHTMLChildren(builder, node)
	if builder.Len() == beforeLen {
		builder.WriteString("\n")
		return
	}
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
}

func writeTelegramListElement(builder *strings.Builder, node *xhtml.Node) {
	if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
	writeTelegramHTMLChildren(builder, node)
}

func writeTelegramListItemElement(builder *strings.Builder, node *xhtml.Node) {
	builder.WriteString("• ")
	writeTelegramHTMLChildren(builder, node)
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
}

func writeTelegramHTMLChildren(builder *strings.Builder, node *xhtml.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeTelegramHTMLNode(builder, child)
	}
}

func telegramAttr(node *xhtml.Node, key string) string {
	value, _ := telegramAttrValue(node, key)
	return value
}

func telegramAttrValue(node *xhtml.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val), true
		}
	}
	return "", false
}
