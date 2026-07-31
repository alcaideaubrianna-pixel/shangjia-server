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
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
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

func buildCollectRuleDecision(event gdb.Record, content *collectContentResult, rule gdb.Record) *collectRuleDecision {
	rawText := strings.TrimSpace(event["raw_text"].String())
	mediaCount := event["media_count"].Int()
	if content != nil {
		rawText = content.RawText
		mediaCount = content.MediaCount
	}
	matchJSON := precheckCollectRuleText(rawText, mediaCount, rule).MatchJSON
	if strings.TrimSpace(rawText) == "" {
		return &collectRuleDecision{
			Matched:   true,
			Text:      "",
			MatchJSON: matchJSON,
		}
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
			MatchJSON: matchJSON,
		}
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
		MatchJSON: matchJSON,
	}
}

func shouldDropCollectStandaloneCodeCaption(text string, mediaCount int) bool {
	text = strings.TrimSpace(text)
	return mediaCount == 1 && collectStandaloneCodeCaptionRule.MatchString(text)
}

type collectDedupePhase string

const (
	collectDedupePhaseEarly collectDedupePhase = "early"
	collectDedupePhasePHash collectDedupePhase = "phash"
)

func (s *sSysPublish) collectDedupeContentByKey(ctx context.Context, event gdb.Record, eventRows gdb.Result) (map[string]gdb.Record, error) {
	keys := make([]string, 0, len(eventRows))
	seen := make(map[string]struct{}, len(eventRows))
	for _, row := range eventRows {
		key := strings.TrimSpace(row["dedupe_key"].String())
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return map[string]gdb.Record{}, nil
	}
	rows, err := pdao.YoubanPublishCollectContent.Ctx(ctx).
		Fields("dedupe_key,text_hash,media_json").
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		WhereIn("dedupe_key", keys).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集内容PHash快照失败")
	}
	contentByKey := make(map[string]gdb.Record, len(rows))
	for _, row := range rows {
		if key := strings.TrimSpace(row["dedupe_key"].String()); key != "" {
			contentByKey[key] = row
		}
	}
	return contentByKey, nil
}

func (s *sSysPublish) collectDedupeCandidateEvents(ctx context.Context, event gdb.Record, current collectDedupeMaterial, days int) (gdb.Result, error) {
	newModel := func() *gdb.Model {
		model := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("tenant_id", event["tenant_id"].Int64()).
			Where("account_id", event["account_id"].Int64()).
			WhereLT("id", event["id"].Int64())
		if days > 0 {
			model = model.WhereGTE("received_at", gtime.NewFromTime(time.Now().AddDate(0, 0, -days)))
		}
		return model
	}

	rowsByID := make(map[int64]gdb.Record)
	appendExactRows := func(field, signature string) error {
		if signature == "" {
			return nil
		}
		exactRows, err := newModel().
			Where(field, signature).
			Fields("id,text_hash,dedupe_key,media_json,received_at").
			OrderDesc("id").
			All()
		if err != nil {
			return err
		}
		for _, row := range exactRows {
			rowsByID[row["id"].Int64()] = row
		}
		return nil
	}
	if err := appendExactRows("dedupe_key", current.dedupeKey); err != nil {
		return nil, err
	}
	if err := appendExactRows("text_hash", current.textHash); err != nil {
		return nil, err
	}
	if current.mediaKey != "" || current.imagePHashKey != "" {
		recentRows, err := newModel().
			Fields("id,text_hash,dedupe_key,media_json,received_at").
			OrderDesc("id").
			Limit(500).
			All()
		if err != nil {
			return nil, err
		}
		for _, row := range recentRows {
			rowsByID[row["id"].Int64()] = row
		}
	}
	rows := make(gdb.Result, 0, len(rowsByID))
	for _, row := range rowsByID {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left]["id"].Int64() > rows[right]["id"].Int64()
	})
	return rows, nil
}

type collectDedupeMaterial struct {
	dedupeKey     string
	mediaKey      string
	textHash      string
	imagePHashKey string
	mediaCount    int
}

func collectDedupeMaterialFromEvent(event gdb.Record, content *collectContentResult) collectDedupeMaterial {
	mediaJSON := event["media_json"].String()
	textHash := strings.TrimSpace(event["text_hash"].String())
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	if content != nil {
		if strings.TrimSpace(content.MediaJSON) != "" {
			mediaJSON = content.MediaJSON
		}
		if strings.TrimSpace(content.TextHash) != "" {
			textHash = content.TextHash
		}
		if strings.TrimSpace(content.DedupeKey) != "" {
			dedupeKey = content.DedupeKey
		}
	}
	material := collectDedupeMaterialFromValues(textHash, mediaJSON)
	material.dedupeKey = dedupeKey
	if content == nil {
		material.imagePHashKey = ""
	}
	return material
}

func collectDedupeMaterialFromEventRecord(row gdb.Record) collectDedupeMaterial {
	material := collectDedupeMaterialFromValues(row["text_hash"].String(), row["media_json"].String())
	material.dedupeKey = strings.TrimSpace(row["dedupe_key"].String())
	material.imagePHashKey = ""
	return material
}

func collectDedupeMaterialFromContentRecord(row gdb.Record) collectDedupeMaterial {
	material := collectDedupeMaterialFromValues(row["text_hash"].String(), row["media_json"].String())
	material.dedupeKey = strings.TrimSpace(row["dedupe_key"].String())
	return material
}

func collectDedupeMaterialFromValues(textHash string, mediaJSON string) collectDedupeMaterial {
	items := make([]collectMediaItem, 0)
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		items = nil
	}
	mediaKey := collectMediaFingerprintSetKey(items)
	imagePHashKey, _ := collectImagePHashSetKey(items)
	textHash = strings.TrimSpace(textHash)
	if textHash == collectHash("") {
		textHash = ""
	}
	return collectDedupeMaterial{
		mediaKey:      mediaKey,
		textHash:      textHash,
		imagePHashKey: imagePHashKey,
		mediaCount:    len(collectMediaFingerprintValues(items)),
	}
}

func (material collectDedupeMaterial) matchLayer(previous collectDedupeMaterial, phase collectDedupePhase) string {
	if phase == collectDedupePhaseEarly {
		if material.mediaKey != "" && material.mediaKey == previous.mediaKey {
			return "media_fingerprint"
		}
		if material.textHash != "" && material.textHash == previous.textHash {
			return "text_hash"
		}
	}
	if phase == collectDedupePhasePHash && material.imagePHashKey != "" && material.imagePHashKey == previous.imagePHashKey {
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
