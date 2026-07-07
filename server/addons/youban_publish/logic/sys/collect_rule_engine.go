package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

var (
	collectLinkPattern     = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern = regexp.MustCompile(`(^|\s)@[A-Za-z0-9_]{4,}`)
)

type collectRuleDecision struct {
	Matched   bool
	Skipped   bool
	Reason    string
	Text      string
	MatchJSON string
}

type collectReplaceRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *sSysPublish) evaluateCollectRule(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record) (*collectRuleDecision, error) {
	rawText := strings.TrimSpace(event["raw_text"].String())
	mediaCount := event["media_count"].Int()
	if content != nil {
		rawText = content.RawText
		mediaCount = content.MediaCount
	}
	if rule["block_plain_text"].Int() == 1 && mediaCount == 0 {
		return skippedCollectRule("纯文本"), nil
	}
	if rule["block_link"].Int() == 1 && collectLinkPattern.MatchString(rawText) {
		return skippedCollectRule("链接"), nil
	}
	if rule["block_username"].Int() == 1 && collectUsernamePattern.MatchString(rawText) {
		return skippedCollectRule("用户名"), nil
	}
	if rule["min_media_count_enabled"].Int() == 1 && mediaCount < rule["min_media_count"].Int() {
		return skippedCollectRule(fmt.Sprintf("媒体数量少于%d", rule["min_media_count"].Int())), nil
	}
	if blockedText := matchCollectTerms(rawText, collectStringList(rule["block_text_json"].String())); blockedText != "" {
		return skippedCollectRule("屏蔽文本:" + blockedText), nil
	}
	matchedKeywords := matchedCollectTerms(rawText, collectStringList(rule["keyword_json"].String()))
	if len(collectStringList(rule["keyword_json"].String())) > 0 && len(matchedKeywords) == 0 {
		return skippedCollectRule("未命中关键词"), nil
	}
	matchedTags := matchedCollectTags(rawText, collectStringList(rule["tag_json"].String()))
	if len(collectStringList(rule["tag_json"].String())) > 0 && len(matchedTags) == 0 {
		return skippedCollectRule("未命中标签"), nil
	}
	if rule["dedupe_enabled"].Int() == 1 {
		duplicated, err := s.collectDuplicated(ctx, event, content, rule["dedupe_days"].Int())
		if err != nil {
			return nil, err
		}
		if duplicated {
			return skippedCollectRule("图文重复"), nil
		}
	}
	text := applyCollectReplacements(rawText, collectReplaceList(rule["replace_json"].String()))
	if rule["header_enabled"].Int() == 1 && strings.TrimSpace(rule["header_markdown"].String()) != "" {
		text = strings.TrimSpace(rule["header_markdown"].String()) + "\n\n" + text
	}
	if rule["footer_enabled"].Int() == 1 && strings.TrimSpace(rule["footer_markdown"].String()) != "" {
		text = strings.TrimSpace(text + "\n\n" + strings.TrimSpace(rule["footer_markdown"].String()))
	}
	if rule["show_unique_no"].Int() == 1 {
		text = fmt.Sprintf("编号: C%010d\n\n%s", event["id"].Int64(), strings.TrimSpace(text))
	}
	matchJSON := collectMatchJSON(matchedKeywords, matchedTags)
	return &collectRuleDecision{
		Matched:   true,
		Text:      strings.TrimSpace(text),
		MatchJSON: matchJSON,
	}, nil
}

func skippedCollectRule(reason string) *collectRuleDecision {
	return &collectRuleDecision{
		Matched:   false,
		Skipped:   true,
		Reason:    reason,
		MatchJSON: collectMatchJSON(nil, nil),
	}
}

func (s *sSysPublish) collectDuplicated(ctx context.Context, event gdb.Record, content *collectContentResult, days int) (bool, error) {
	if days <= 0 || days > 7 {
		days = 7
	}
	since := gtime.NewFromTime(time.Now().AddDate(0, 0, -days))
	if content != nil && content.PreviousSeenAt != nil {
		return !content.PreviousSeenAt.Before(since), nil
	}
	mod := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		WhereLT("id", event["id"].Int64()).
		WhereGTE("received_at", since)
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	textHash := strings.TrimSpace(event["text_hash"].String())
	switch {
	case dedupeKey != "" && textHash != "":
		mod = mod.Where("(dedupe_key=? OR text_hash=?)", dedupeKey, textHash)
	case dedupeKey != "":
		mod = mod.Where("dedupe_key", dedupeKey)
	case textHash != "":
		mod = mod.Where("text_hash", textHash)
	default:
		return false, nil
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "采集去重判断失败")
	}
	return count > 0, nil
}

func collectStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return trimCollectValues(values)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err == nil {
		values = make([]string, 0, len(rows))
		for _, row := range rows {
			for _, key := range []string{"value", "label", "text", "keyword", "tag"} {
				if value := strings.TrimSpace(fmt.Sprint(row[key])); value != "" && value != "<nil>" {
					values = append(values, value)
					break
				}
			}
		}
		return trimCollectValues(values)
	}
	return trimCollectValues(strings.Split(raw, ","))
}

func trimCollectValues(values []string) []string {
	list := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		list = append(list, value)
	}
	return list
}

func collectReplaceList(raw string) []collectReplaceRule {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var rows []collectReplaceRule
	if err := json.Unmarshal([]byte(raw), &rows); err == nil {
		return rows
	}
	var maps []map[string]string
	if err := json.Unmarshal([]byte(raw), &maps); err != nil {
		return nil
	}
	rows = make([]collectReplaceRule, 0, len(maps))
	for _, row := range maps {
		rows = append(rows, collectReplaceRule{
			From: firstCollectValue(row, "from", "source", "match", "old"),
			To:   firstCollectValue(row, "to", "target", "replace", "new"),
		})
	}
	return rows
}

func firstCollectValue(row map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(row[key]); value != "" {
			return value
		}
	}
	return ""
}

func matchCollectTerms(text string, terms []string) string {
	matched := matchedCollectTerms(text, terms)
	if len(matched) == 0 {
		return ""
	}
	return matched[0]
}

func matchedCollectTerms(text string, terms []string) []string {
	lowerText := strings.ToLower(text)
	list := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.Contains(lowerText, strings.ToLower(term)) {
			list = append(list, term)
		}
	}
	return list
}

func matchedCollectTags(text string, tags []string) []string {
	lowerText := strings.ToLower(text)
	list := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(tag)), "#")
		if normalized == "" {
			continue
		}
		if strings.Contains(lowerText, "#"+normalized) || strings.Contains(lowerText, normalized) {
			list = append(list, tag)
		}
	}
	return list
}

func applyCollectReplacements(text string, rules []collectReplaceRule) string {
	for _, rule := range rules {
		if strings.TrimSpace(rule.From) == "" {
			continue
		}
		text = strings.ReplaceAll(text, rule.From, rule.To)
	}
	return text
}

func collectMatchJSON(keywords []string, tags []string) string {
	data, _ := json.Marshal(map[string][]string{
		"keywords": keywords,
		"tags":     tags,
	})
	return string(data)
}
