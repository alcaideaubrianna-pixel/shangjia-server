package sys

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/database/gdb"
)

var (
	collectLinkPattern               = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern           = regexp.MustCompile(`@`)
	collectStandaloneCodeCaptionRule = regexp.MustCompile(`^[A-Za-z]{1,4}\d{3,6}$`)
	collectMaterialMetaLineRule      = regexp.MustCompile(`^\s*(?:昵称|编号|同行)\s*(?:[:：=].*)?\s*$`)
	collectMaterialCodeLineRule      = regexp.MustCompile(`^\s*(?:(?:[A-Za-z]{1,8}[-_ ]?\d{3,10}|[\p{Han}]{1,8}\d{3,10})(?:\s+|$))+\s*$`)
	collectIntroFeeAmountRule        = regexp.MustCompile(`(?:(?:介绍|推荐|牵线|居间|对接)费(?:用)?|中介(?:服务)?费(?:用)?)\s*[:：=]?\s*([¥￥]?\s*\d[\d,.]*(?:\s*(?:元|万))?)`)
	collectIntroFeeStandaloneSuffix  = regexp.MustCompile(`^[\p{Han}A-Za-z0-9]{1,32}$`)
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
	text := rawText
	introFeeSuffix := strings.TrimSpace(rule["intro_fee_suffix"].String())
	if rule["truncate_intro_fee_enabled"].Bool() {
		text = applyCollectIntroFeeTruncate(text)
	}
	text = applyCollectLineDeletes(text, collectRuleStrings(rule, "delete_lines"))
	text = applyCollectTextDeletes(text, collectRuleStrings(rule, "delete_texts"))
	text = applyCollectReplacements(text, collectRuleReplacements(rule))
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
		text = strings.TrimSpace(text + "\n" + strings.TrimSpace(rule["footer_markdown"].String()))
	}
	if introFeeSuffix != "" {
		text = applyCollectIntroFeeSuffix(text, rawText, introFeeSuffix)
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

type collectDedupeMaterial struct {
	dedupeKey       string
	mediaKey        string
	textHash        string
	imagePHashKey   string
	mediaCount      int
	mediaTotal      int
	imagePHashCount int
	imageTotal      int
}

func collectDedupeMaterialFromEvent(event gdb.Record, content *collectContentResult) collectDedupeMaterial {
	items := make([]collectMediaItem, 0)
	textHash := strings.TrimSpace(event["text_hash"].String())
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	if content != nil {
		if len(content.Media) > 0 {
			items = content.Media
		}
		if strings.TrimSpace(content.TextHash) != "" {
			textHash = content.TextHash
		}
		if strings.TrimSpace(content.DedupeKey) != "" {
			dedupeKey = content.DedupeKey
		}
	}
	material := collectDedupeMaterialFromItems(textHash, items)
	material.dedupeKey = dedupeKey
	if content == nil {
		material.imagePHashKey = ""
	}
	return material
}

func collectDedupeMaterialFromEventRecord(row gdb.Record, items []collectMediaItem) collectDedupeMaterial {
	material := collectDedupeMaterialFromItems(row["text_hash"].String(), items)
	material.dedupeKey = strings.TrimSpace(row["dedupe_key"].String())
	return material
}

func collectDedupeMaterialFromItems(textHash string, items []collectMediaItem) collectDedupeMaterial {
	mediaKey := collectMediaFingerprintSetKey(items)
	imagePHashKey, imagePHashCount := collectImagePHashSetKey(items)
	imageTotal := 0
	for _, item := range items {
		if mediaType := strings.ToLower(strings.TrimSpace(item.Type)); mediaType == "photo" || mediaType == "image" {
			imageTotal++
		}
	}
	mediaCount := len(collectMediaFingerprintValues(items))
	if mediaCount != len(items) {
		mediaKey = ""
	}
	if imagePHashCount != imageTotal {
		imagePHashKey = ""
	}
	textHash = strings.TrimSpace(textHash)
	if textHash == collectHash("") {
		textHash = ""
	}
	return collectDedupeMaterial{
		mediaKey:        mediaKey,
		textHash:        textHash,
		imagePHashKey:   imagePHashKey,
		mediaCount:      mediaCount,
		mediaTotal:      len(items),
		imagePHashCount: imagePHashCount,
		imageTotal:      imageTotal,
	}
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

func applyCollectIntroFeeTruncate(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	changed := false
	cutoff := len(lines)
	for index, line := range lines {
		// Match against a normalized copy only; preserve the original text,
		// emoji, spacing, and formatting in all retained lines.
		if collectIntroFeeAmount(line) != "" {
			cutoff = index
			changed = true
			break
		}
	}
	lines = lines[:cutoff]
	start := 0
	firstContentLine := true
	for start < len(lines) {
		line := strings.TrimSpace(lines[start])
		if line == "" || collectMaterialMetaLineRule.MatchString(line) || collectMaterialCodeLineRule.MatchString(line) || (firstContentLine && !collectLineContainsChinese(line)) {
			start++
			changed = true
			continue
		}
		firstContentLine = false
		break
	}
	kept := make([]string, 0, len(lines)-start)
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if collectMaterialMetaLineRule.MatchString(trimmed) || isCollectIntroFeeSourceMarkLine(trimmed) {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if changed {
		return strings.TrimSpace(strings.Join(kept, "\n"))
	}
	return text
}

func isCollectIntroFeeSourceMarkLine(line string) bool {
	return strings.Contains(normalizeCollectKeywordText(line), "情诗")
}

func applyCollectIntroFeeSuffix(text, original, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	amount := collectIntroFeeAmount(original)
	if suffix == "" || amount == "" {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines)+1)
	removeNextSuffix := false
	for _, line := range lines {
		if removeNextSuffix && isCollectIntroFeeStandaloneSuffix(strings.TrimSpace(line)) {
			removeNextSuffix = false
			continue
		}
		removeNextSuffix = false
		if collectIntroFeeAmount(normalizeCollectKeywordText(line)) != "" {
			removeNextSuffix = true
			continue
		}
		kept = append(kept, line)
	}
	body := strings.TrimSpace(strings.Join(kept, "\n"))
	feeLine := "介绍费 " + amount + " " + suffix
	if body == "" {
		return feeLine
	}
	return body + "\n" + feeLine
}

func isCollectIntroFeeStandaloneSuffix(text string) bool {
	text = strings.TrimSpace(text)
	if !collectIntroFeeStandaloneSuffix.MatchString(text) {
		return false
	}
	for _, r := range text {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

func collectIntroFeeAmount(text string) string {
	match := collectIntroFeeAmountRule.FindStringSubmatch(normalizeCollectKeywordText(text))
	if len(match) < 2 {
		return ""
	}
	return strings.Join(strings.Fields(strings.TrimSpace(match[1])), " ")
}

func normalizeCollectKeywordText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || isCollectInvisibleVariationSelector(r) {
			return -1
		}
		return r
	}, text)
}

func isCollectInvisibleVariationSelector(r rune) bool {
	return (r >= '\u180b' && r <= '\u180d') ||
		(r >= '\ufe00' && r <= '\ufe0f') ||
		(r >= '\U000e0100' && r <= '\U000e01ef')
}

func collectLineContainsChinese(line string) bool {
	for _, char := range line {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
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
