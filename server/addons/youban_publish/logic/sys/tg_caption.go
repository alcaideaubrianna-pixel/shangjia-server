package sys

import (
	"context"
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	xhtml "golang.org/x/net/html"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) telegramJobText(ctx context.Context, taskId int64) (string, error) {
	row, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return "", err
	}
	setting, err := s.accountSetting(ctx, row["tenant_id"].Int64(), row["account_id"].Int64())
	if err != nil {
		return "", err
	}
	return buildTelegramTaskCaption(row, setting), nil
}

func buildTelegramTaskCaption(row gdb.Record, setting *sysin.AccountSettingModel) string {
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
		if suffix := telegramSuffixHTML(setting.SuffixContent); suffix != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, suffix)
		}
	}
	return strings.Join(lines, "\n")
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
	if title := strings.TrimSpace(row["title"].String()); title != "" {
		return title
	}
	number := telegramCaptionNumber(row)
	if number == "" {
		return ""
	}
	if setting.NumberSource == "random" {
		return number
	}
	prefix := strings.TrimSpace(setting.CustomMarkText)
	if setting.MarkMode != "custom" || prefix == "" {
		prefix = strings.TrimSpace(row["account_nickname"].String())
	}
	if prefix == "" {
		return number
	}
	return fmt.Sprintf("%s%s", prefix, number)
}

func telegramCaptionNumber(row gdb.Record) string {
	if profileNo := strings.TrimSpace(row["profile_no"].String()); profileNo != "" {
		return profileNo
	}
	return ""
}

func telegramEscapeText(value string) string {
	return stdhtml.EscapeString(strings.TrimSpace(value))
}

func telegramSuffixHTML(value string) string {
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
	href := ""
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "href") {
			href = strings.TrimSpace(attr.Val)
			break
		}
	}
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
	writeTelegramHTMLChildren(builder, node)
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
