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
	"hotgo/addons/youban_publish/model/input/sysin"
)

var (
	collectLinkPattern               = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern           = regexp.MustCompile(`(^|\s)@[A-Za-z0-9_]{4,}`)
	collectStandaloneCodeCaptionRule = regexp.MustCompile(`^[A-Za-z]{1,4}\d{3,6}$`)
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
	precheck := precheckCollectRuleText(rawText, mediaCount, rule)
	if !precheck.Matched {
		return skippedCollectRule(precheck.Reason), nil
	}
	if rule["dedupe_enabled"].Int() == 1 {
		duplicated, err := s.collectDuplicated(ctx, event, content, rule, rule["dedupe_days"].Int())
		if err != nil {
			return nil, err
		}
		if duplicated {
			return skippedCollectRule("图文重复"), nil
		}
	}
	if strings.TrimSpace(rawText) == "" {
		return &collectRuleDecision{
			Matched:   true,
			Text:      "",
			MatchJSON: precheck.MatchJSON,
		}, nil
	}
	text := applyCollectTextDeletes(rawText, collectStringList(rule["delete_text_json"].String()))
	text = applyCollectReplacements(text, collectReplaceList(rule["replace_json"].String()))
	if shouldDropCollectStandaloneCodeCaption(text, mediaCount) {
		text = ""
	}
	if strings.TrimSpace(text) == "" {
		return &collectRuleDecision{
			Matched:   true,
			Text:      "",
			MatchJSON: precheck.MatchJSON,
		}, nil
	}
	if rule["header_enabled"].Int() == 1 && strings.TrimSpace(rule["header_markdown"].String()) != "" {
		text = strings.TrimSpace(rule["header_markdown"].String()) + "\n\n" + text
	}
	if rule["footer_enabled"].Int() == 1 && strings.TrimSpace(rule["footer_markdown"].String()) != "" {
		text = strings.TrimSpace(text + "\n\n" + strings.TrimSpace(rule["footer_markdown"].String()))
	}
	if rule["show_unique_no"].Int() == 1 {
		text = fmt.Sprintf("编号: C%010d\n\n%s", event["id"].Int64(), strings.TrimSpace(text))
	}
	return &collectRuleDecision{
		Matched:   true,
		Text:      strings.TrimSpace(text),
		MatchJSON: precheck.MatchJSON,
	}, nil
}

func shouldDropCollectStandaloneCodeCaption(text string, mediaCount int) bool {
	text = strings.TrimSpace(text)
	return mediaCount == 1 && collectStandaloneCodeCaptionRule.MatchString(text)
}

func skippedCollectRule(reason string) *collectRuleDecision {
	return &collectRuleDecision{
		Matched:   false,
		Skipped:   true,
		Reason:    reason,
		MatchJSON: collectMatchJSON(nil, nil),
	}
}

func (s *sSysPublish) collectDuplicated(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, days int) (bool, error) {
	if days <= 0 || days > 7 {
		days = 7
	}
	since := gtime.NewFromTime(time.Now().AddDate(0, 0, -days))
	mod := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		WhereLT("id", event["id"].Int64()).
		WhereGTE("received_at", since)
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	textHash := strings.TrimSpace(event["text_hash"].String())
	mediaCount := event["media_count"].Int()
	if content != nil {
		mediaCount = content.MediaCount
	}
	switch {
	case mediaCount > 0 && dedupeKey != "":
		mod = mod.Where("dedupe_key", dedupeKey)
	case mediaCount <= 0 && textHash != "" && textHash != collectHash(""):
		mod = mod.Where("text_hash", textHash)
	default:
		return false, nil
	}
	rows, err := mod.Fields("id").Limit(200).All()
	if err != nil {
		return false, gerror.Wrap(err, "采集去重判断失败")
	}
	targetIds := decodeInt64JSON(rule["target_channel_id_json"].String())
	if len(targetIds) == 0 {
		return false, nil
	}
	for _, row := range rows {
		if duplicated, err := s.collectEventDuplicatedInTargets(ctx, row["id"].Int64(), targetIds); err != nil {
			return false, err
		} else if duplicated {
			return true, nil
		}
	}
	return false, nil
}

func (s *sSysPublish) collectEventDuplicatedInTargets(ctx context.Context, eventId int64, targetIds []int64) (bool, error) {
	if eventId <= 0 || len(targetIds) == 0 {
		return false, nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("target_channel_id_json,status").
		Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusSent}).
		All()
	if err != nil {
		return false, gerror.Wrap(err, "读取采集去重分发记录失败")
	}
	for _, row := range rows {
		if int64SlicesOverlap(targetIds, decodeInt64JSON(row["target_channel_id_json"].String())) {
			return true, nil
		}
	}
	return false, nil
}

func int64SlicesOverlap(left []int64, right []int64) bool {
	seen := make(map[int64]struct{}, len(left))
	for _, value := range left {
		if value > 0 {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
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

func applyCollectTextDeletes(text string, values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		text = strings.ReplaceAll(text, value, "")
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
