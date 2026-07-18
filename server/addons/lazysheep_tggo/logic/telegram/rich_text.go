// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"html"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"
)

type richEntity struct {
	models.MessageEntity
	Start int
	End   int
}

func telegramMessageHTML(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	return telegramTextHTML(msg.Text, msg.Entities)
}

func telegramTextHTML(text string, entities []models.MessageEntity) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if len(entities) == 0 {
		return html.EscapeString(text)
	}
	runes := []rune(text)
	items := normalizeRichEntities(runes, entities)
	if len(items) == 0 {
		return html.EscapeString(text)
	}
	return renderRichRange(runes, 0, len(runes), items)
}

func normalizeRichEntities(runes []rune, entities []models.MessageEntity) []richEntity {
	items := make([]richEntity, 0, len(entities))
	for _, entity := range entities {
		start := utf16OffsetToRuneIndex(runes, entity.Offset)
		end := utf16OffsetToRuneIndex(runes, entity.Offset+entity.Length)
		if start < 0 || end <= start || end > len(runes) {
			continue
		}
		items = append(items, richEntity{MessageEntity: entity, Start: start, End: end})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Start != items[j].Start {
			return items[i].Start < items[j].Start
		}
		return items[i].End > items[j].End
	})
	return items
}

func utf16OffsetToRuneIndex(runes []rune, offset int) int {
	if offset < 0 {
		return -1
	}
	units := 0
	for i, r := range runes {
		if units == offset {
			return i
		}
		units += len(utf16.Encode([]rune{r}))
		if units > offset {
			return -1
		}
	}
	if units == offset {
		return len(runes)
	}
	return -1
}

func renderRichRange(runes []rune, start, end int, entities []richEntity) string {
	var out strings.Builder
	pos := start
	for i := 0; i < len(entities); i++ {
		entity := entities[i]
		if entity.Start < start || entity.End > end || entity.End <= pos {
			continue
		}
		if entity.Start > pos {
			out.WriteString(html.EscapeString(string(runes[pos:entity.Start])))
		}
		children := make([]richEntity, 0)
		for j := i + 1; j < len(entities); j++ {
			child := entities[j]
			if child.Start >= entity.End {
				break
			}
			if child.Start >= entity.Start && child.End <= entity.End {
				children = append(children, child)
			}
		}
		inner := renderRichRange(runes, entity.Start, entity.End, children)
		out.WriteString(wrapRichEntity(entity.MessageEntity, inner))
		pos = entity.End
	}
	if pos < end {
		out.WriteString(html.EscapeString(string(runes[pos:end])))
	}
	return out.String()
}

func wrapRichEntity(entity models.MessageEntity, inner string) string {
	switch entity.Type {
	case models.MessageEntityTypeBold:
		return "<b>" + inner + "</b>"
	case models.MessageEntityTypeItalic:
		return "<i>" + inner + "</i>"
	case models.MessageEntityTypeUnderline:
		return "<u>" + inner + "</u>"
	case models.MessageEntityTypeStrikethrough:
		return "<s>" + inner + "</s>"
	case models.MessageEntityTypeSpoiler:
		return "<tg-spoiler>" + inner + "</tg-spoiler>"
	case models.MessageEntityTypeCode:
		return "<code>" + inner + "</code>"
	case models.MessageEntityTypePre:
		if strings.TrimSpace(entity.Language) != "" {
			return `<pre><code class="language-` + html.EscapeString(entity.Language) + `">` + inner + "</code></pre>"
		}
		return "<pre>" + inner + "</pre>"
	case models.MessageEntityTypeTextLink:
		if strings.TrimSpace(entity.URL) == "" {
			return inner
		}
		return `<a href="` + html.EscapeString(entity.URL) + `">` + inner + "</a>"
	case models.MessageEntityTypeURL:
		url := strings.TrimSpace(stripHTMLTags(inner))
		if url == "" {
			return inner
		}
		return `<a href="` + html.EscapeString(url) + `">` + inner + "</a>"
	case models.MessageEntityTypeBlockquote:
		return "<blockquote>" + inner + "</blockquote>"
	case models.MessageEntityTypeExpandableBlockquote:
		return "<blockquote expandable>" + inner + "</blockquote>"
	default:
		return inner
	}
}

func stripHTMLTags(text string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'")
	return replacer.Replace(text)
}
