package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var (
	collectLinkPattern               = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern           = regexp.MustCompile(`@`)
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
	text := applyCollectLineDeletes(rawText, collectStringList(rule["delete_line_text_json"].String()))
	text = applyCollectTextDeletes(text, collectStringList(rule["delete_text_json"].String()))
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
	return &collectRuleDecision{
		Matched:   true,
		Text:      strings.TrimSpace(text),
		MatchJSON: precheck.MatchJSON,
	}, nil
}

func (s *sSysPublish) filterCollectRulesByEarlyDedupe(ctx context.Context, event gdb.Record, rules []gdb.Record) ([]gdb.Record, []string, error) {
	if len(rules) == 0 {
		return nil, nil, nil
	}
	filtered := make([]gdb.Record, 0, len(rules))
	reasons := make([]string, 0)
	for _, rule := range rules {
		if rule["dedupe_enabled"].Int() != 1 {
			filtered = append(filtered, rule)
			continue
		}
		duplicated, err := s.collectDuplicatedAtQueueFront(ctx, event, rule, rule["dedupe_days"].Int())
		if err != nil {
			return nil, nil, err
		}
		if duplicated {
			reasons = append(reasons, "图文重复")
			g.Log().Infof(ctx, "采集队列入口去重跳过 eventId:%d ruleId:%d", event["id"].Int64(), rule["id"].Int64())
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered, uniqueStrings(reasons), nil
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
	return s.collectDuplicatedWithPHash(ctx, event, content, rule, days, true)
}

func (s *sSysPublish) collectDuplicatedAtQueueFront(ctx context.Context, event gdb.Record, rule gdb.Record, days int) (bool, error) {
	return s.collectDuplicatedWithPHash(ctx, event, nil, rule, days, false)
}

func (s *sSysPublish) collectDuplicatedWithPHash(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, days int, includePHash bool) (bool, error) {
	targetIds := decodeInt64JSON(rule["target_channel_id_json"].String())
	if len(targetIds) == 0 {
		return false, nil
	}
	current := collectDedupeMaterialFromEvent(event, content)
	if !includePHash {
		current.imagePHashKey = ""
	}
	if current.mediaKey == "" && current.textHash == "" && current.imagePHashKey == "" {
		return false, nil
	}
	mod := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		WhereLT("id", event["id"].Int64())
	if days > 0 {
		mod = mod.WhereGTE("received_at", gtime.NewFromTime(time.Now().AddDate(0, 0, -days)))
	}
	rows, err := mod.Fields("id,text_hash,media_json").OrderDesc("id").Limit(500).All()
	if err != nil {
		return false, gerror.Wrap(err, "采集去重判断失败")
	}
	if len(rows) == 0 {
		return false, nil
	}
	eventIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			eventIds = append(eventIds, id)
		}
	}
	dispatchRows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id,target_channel_id_json").
		WhereIn("event_id", eventIds).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusSent}).
		All()
	if err != nil {
		return false, gerror.Wrap(err, "读取采集去重分发记录失败")
	}
	channelsByEvent := make(map[int64][]int64, len(dispatchRows))
	for _, row := range dispatchRows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		channelsByEvent[eventId] = append(channelsByEvent[eventId], decodeInt64JSON(row["target_channel_id_json"].String())...)
	}
	for _, row := range rows {
		previousEventId := row["id"].Int64()
		channelId := firstOverlappingInt64(targetIds, channelsByEvent[previousEventId])
		if channelId <= 0 {
			continue
		}
		previous := collectDedupeMaterialFromRecord(row)
		if !includePHash {
			previous.imagePHashKey = ""
		}
		layer := current.matchLayer(previous)
		if layer == "" {
			continue
		}
		g.Log().Infof(ctx, "采集去重命中 eventId:%d previousEventId:%d channelId:%d layer:%s currentMedia:%d previousMedia:%d", event["id"].Int64(), previousEventId, channelId, layer, current.mediaCount, previous.mediaCount)
		return true, nil
	}
	return false, nil
}

type collectDedupeMaterial struct {
	mediaKey        string
	textHash        string
	imagePHashKey   string
	mediaCount      int
	imagePHashCount int
}

func collectDedupeMaterialFromEvent(event gdb.Record, content *collectContentResult) collectDedupeMaterial {
	mediaJSON := event["media_json"].String()
	textHash := strings.TrimSpace(event["text_hash"].String())
	if content != nil {
		if strings.TrimSpace(content.MediaJSON) != "" {
			mediaJSON = content.MediaJSON
		}
		if strings.TrimSpace(content.TextHash) != "" {
			textHash = content.TextHash
		}
	}
	return collectDedupeMaterialFromValues(textHash, mediaJSON)
}

func collectDedupeMaterialFromRecord(row gdb.Record) collectDedupeMaterial {
	return collectDedupeMaterialFromValues(row["text_hash"].String(), row["media_json"].String())
}

func collectDedupeMaterialFromValues(textHash string, mediaJSON string) collectDedupeMaterial {
	items := make([]collectMediaItem, 0)
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		items = nil
	}
	mediaKey := collectMediaFingerprintSetKey(items)
	imagePHashKey, imagePHashCount := collectImagePHashSetKey(items)
	textHash = strings.TrimSpace(textHash)
	if textHash == collectHash("") {
		textHash = ""
	}
	return collectDedupeMaterial{
		mediaKey:        mediaKey,
		textHash:        textHash,
		imagePHashKey:   imagePHashKey,
		mediaCount:      len(collectMediaFingerprintValues(items)),
		imagePHashCount: imagePHashCount,
	}
}

func (material collectDedupeMaterial) matchLayer(previous collectDedupeMaterial) string {
	if material.mediaKey != "" && material.mediaKey == previous.mediaKey {
		return "media_fingerprint"
	}
	if material.textHash != "" && material.textHash == previous.textHash {
		return "text_hash"
	}
	if material.imagePHashKey != "" && material.imagePHashKey == previous.imagePHashKey {
		return "image_phash"
	}
	return ""
}

func collectMediaFingerprintSetKey(items []collectMediaItem) string {
	values := collectMediaFingerprintValues(items)
	if len(values) == 0 {
		return ""
	}
	return collectHash(strings.Join(values, "|"))
}

func collectMediaFingerprintValues(items []collectMediaItem) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		fingerprint := strings.TrimSpace(collectMediaFingerprint(item))
		if fingerprint == "" {
			continue
		}
		values = append(values, strings.ToLower(strings.TrimSpace(item.Type))+":"+fingerprint)
	}
	sort.Strings(values)
	return values
}

func collectImagePHashSetKey(items []collectMediaItem) (string, int) {
	values := make([]string, 0, len(items))
	for _, item := range items {
		mediaType := strings.ToLower(strings.TrimSpace(item.Type))
		if mediaType != "photo" && mediaType != "image" {
			continue
		}
		if hash := collectMediaPHash(item); hash != "" {
			values = append(values, hash)
		}
	}
	sort.Strings(values)
	if len(values) == 0 {
		return "", 0
	}
	return collectHash(strings.Join(values, "|")), len(values)
}

func firstOverlappingInt64(left []int64, right []int64) int64 {
	seen := make(map[int64]struct{}, len(left))
	for _, value := range left {
		if value > 0 {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return value
		}
	}
	return 0
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

func applyCollectLineDeletes(text string, values []string) string {
	keywords := normalizedCollectTerms(values)
	if len(keywords) == 0 {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if matchCollectTerms(line, keywords) != "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func normalizedCollectTerms(values []string) []string {
	list := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			list = append(list, value)
		}
	}
	return list
}

func collectMatchJSON(keywords []string, tags []string) string {
	data, _ := json.Marshal(map[string][]string{
		"keywords": keywords,
		"tags":     tags,
	})
	return string(data)
}
