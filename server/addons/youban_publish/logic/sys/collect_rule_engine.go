package sys

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/database/gdb"
	"golang.org/x/text/unicode/norm"
)

var (
	collectLinkPattern               = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern           = regexp.MustCompile(`@`)
	collectStandaloneCodeCaptionRule = regexp.MustCompile(`^[A-Za-z]{1,4}\d{3,6}$`)
	collectMaterialMetaLineRule      = regexp.MustCompile(`^\s*(?:标题|昵称|编号|编码|同行)\s*(?:[:：=].*)?\s*$`)
	collectMaterialCodeLineRule      = regexp.MustCompile(`^\s*(?:(?:[A-Za-z]{1,8}[-_ ]?\d{3,10}|[\p{Han}]{1,8}\d{3,10})(?:\s+|$))+\s*$`)
	collectIntroFeeAmountRule        = regexp.MustCompile(`(?:(?:介绍|推荐|牵线|居间|对接|感谢)费(?:用)?|大米|中介(?:服务)?费(?:用)?)[\s\p{Zs}]*[:：=；;，,]?[\s\p{Zs}]*([¥￥]?[\s\p{Zs}]*\d[\d,.]*(?:[\s\p{Zs}]*(?:元|万))?)`)
	collectIntroFeeLooseAmountRule   = regexp.MustCompile(`(?:(?:介绍|推荐|牵线|居间|对接|感谢)费(?:用)?|大米|中介(?:服务)?费(?:用)?)[^0-9０-９]{0,32}([¥￥]?[\s\p{Zs}]*[0-9０-９][0-9０-９,，.]*)`)
	collectNumberRule                = regexp.MustCompile(`[0-9０-９]+`)
	collectIntroFeeStandaloneSuffix  = regexp.MustCompile(`^[\p{Han}A-Za-z0-9]{1,32}$`)
	collectFeeKeywordPattern         = regexp.MustCompile(`(?:介绍|推荐|牵线|居间|对接|感谢)费|大米`)
)

type collectRuleDecision struct {
	Matched   bool
	Skipped   bool
	Reason    string
	Text      string
	MatchJSON string
}

type collectIntroFeeFields struct {
	FirstAmount   string
	FirstSuffix   string
	ForeignAmount string
}

func parseCollectIntroFeeFields(text string) collectIntroFeeFields {
	clean := normalizeCollectKeywordText(text)
	// Only numbers on the fee line are fee amounts. Other profile fields may
	// contain ages, IDs, heights, etc. and must never become domestic/foreign fees.
	feeLineFound := false
	for _, line := range strings.Split(clean, "\n") {
		if collectFeeKeywordPattern.MatchString(line) {
			clean = strings.TrimSpace(line)
			feeLineFound = true
			break
		}
	}
	fields := collectIntroFeeFields{}
	if !feeLineFound {
		return fields
	}
	amounts := collectNumberRule.FindAllString(clean, -1)
	if len(amounts) > 0 {
		fields.FirstAmount = strings.Join(strings.Fields(amounts[0]), " ")
	}
	if len(amounts) > 1 && strings.Contains(clean, "国外") {
		fields.ForeignAmount = strings.Join(strings.Fields(amounts[1]), " ")
	}
	if fields.FirstAmount != "" {
		feeKeyword := collectFeeKeywordPattern.FindString(clean)
		if feeIndex := strings.Index(clean, feeKeyword); feeKeyword != "" && feeIndex >= 0 && feeIndex+len(feeKeyword) < len(clean) {
			afterFee := strings.TrimSpace(strings.TrimLeft(clean[feeIndex+len(feeKeyword):], "：:；;，,./"))
			afterFee = strings.TrimSpace(strings.TrimLeft(afterFee, "：:；;，,./"))
			if afterFee != "" && !strings.Contains(afterFee, fields.FirstAmount) {
				fields.FirstSuffix = afterFee
			}
		}
		if index := strings.Index(clean, fields.FirstAmount); index > 0 {
			prefix := strings.TrimSpace(clean[:index])
			prefix = strings.TrimSpace(strings.TrimLeft(prefix, "：:；;，,./"))
			prefix = strings.TrimSpace(strings.TrimLeft(prefix, "：:；;，,./"))
			prefix = strings.TrimSpace(strings.TrimPrefix(prefix, feeKeyword))
			prefix = strings.TrimSpace(strings.TrimLeft(prefix, "：:；;，,./"))
			prefix = strings.TrimSpace(strings.TrimRight(prefix, "：:；;，,./"))
			if prefix != "" && !strings.Contains(prefix, "国内") && !strings.Contains(prefix, "国外") {
				fields.FirstSuffix = prefix
			}
		}
	}
	return fields
}

// authoritativeCollectRawText removes an already-processed copy that can be
// appended to a collected message as "<source> | <processed>". The event raw
// text remains unchanged for audit; this helper is only used as the input to
// rule processing so retries cannot process the generated copy as a second
// source document.
func authoritativeCollectRawText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	parts := strings.Split(text, " | ")
	if len(parts) < 2 {
		return text
	}
	for i := 1; i < len(parts); i++ {
		left := strings.TrimSpace(strings.Join(parts[:i], " | "))
		right := strings.TrimSpace(strings.Join(parts[i:], " | "))
		if collectFeeKeywordPattern.MatchString(left) && collectFeeKeywordPattern.MatchString(right) {
			return left
		}
	}
	return text
}

func formatCollectIntroFeeFields(fields collectIntroFeeFields, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if fields.FirstAmount == "" {
		return ""
	}
	if fields.ForeignAmount != "" {
		return strings.TrimSpace(fmt.Sprintf("介绍费 %s 国外 %s %s", fields.FirstAmount, fields.ForeignAmount, suffix))
	}
	return strings.TrimSpace(fmt.Sprintf("介绍费 %s %s", fields.FirstAmount, suffix))
}

type collectReplaceRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func buildCollectRuleDecision(event gdb.Record, content *collectContentResult, rule gdb.Record) *collectRuleDecision {
	rawText := authoritativeCollectRawText(event["raw_text"].String())
	mediaCount := event["media_count"].Int()
	if content != nil {
		rawText = authoritativeCollectRawText(content.RawText)
		mediaCount = content.MediaCount
	}
	precheck := precheckCollectRuleText(rawText, mediaCount, rule)
	if !precheck.Matched {
		return &collectRuleDecision{Skipped: true, Reason: precheck.Reason, MatchJSON: precheck.MatchJSON}
	}
	matchJSON := precheck.MatchJSON
	if strings.TrimSpace(rawText) == "" {
		return &collectRuleDecision{
			Matched:   true,
			Text:      "",
			MatchJSON: matchJSON,
		}
	}
	introFeeSuffix := strings.TrimSpace(rule["intro_fee_suffix"].String())
	text := normalizeCollectRuleBody(rawText, rule)
	if shouldDropCollectStandaloneCodeCaption(text, mediaCount) {
		text = ""
	}
	// Keep evaluating suffix reconstruction even when truncation removed the
	// entire body; the fee line itself must be restored from the pre-truncate text.
	if strings.TrimSpace(text) == "" {
		if introFeeSuffix == "" {
			return &collectRuleDecision{Matched: true, Text: "", MatchJSON: matchJSON}
		}
		candidate := normalizeCollectRuleBodyWithoutTruncate(rawText, rule)
		if strings.Contains(candidate, "\nKK") {
			return &collectRuleDecision{Matched: true, Text: "", MatchJSON: matchJSON}
		}
		meaningful := false
		feeSeen := false
		for _, line := range strings.Split(strings.ReplaceAll(candidate, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if collectFeeKeywordPattern.MatchString(line) {
				feeSeen = true
				continue
			}
			if feeSeen && (isCollectIntroFeeStandaloneSuffix(line) || collectShortTitleLine(line)) {
				return &collectRuleDecision{Matched: true, Text: "", MatchJSON: matchJSON}
			}
			if line == "" || collectMaterialMetaLineRule.MatchString(line) {
				continue
			}
			meaningful = true
			break
		}
		if !meaningful {
			return &collectRuleDecision{Matched: true, Text: "", MatchJSON: matchJSON}
		}
	}
	if rule["header_enabled"].Int() == 1 && strings.TrimSpace(rule["header_markdown"].String()) != "" {
		text = strings.TrimSpace(rule["header_markdown"].String()) + "\n\n" + text
	}
	if rule["footer_enabled"].Int() == 1 && strings.TrimSpace(rule["footer_markdown"].String()) != "" {
		text = strings.TrimSpace(text + "\n" + strings.TrimSpace(rule["footer_markdown"].String()))
	}
	if introFeeSuffix != "" {
		// Suffix reconstruction needs the amount from the cleaned source before
		// truncation removes the original fee line.
		amountText := normalizeCollectRuleBodyWithoutTruncate(authoritativeCollectRawText(rawText), rule)
		text = applyCollectIntroFeeSuffix(text, amountText, introFeeSuffix)
	}
	return &collectRuleDecision{
		Matched:   true,
		Text:      strings.TrimSpace(text),
		MatchJSON: matchJSON,
	}
}

func normalizeCollectRuleBodyWithoutTruncate(text string, rule gdb.Record) string {
	text = applyCollectLineDeletes(text, collectRuleStrings(rule, "delete_lines"))
	text = applyCollectTextDeletes(text, collectRuleStrings(rule, "delete_texts"))
	return applyCollectReplacements(text, collectRuleReplacements(rule))
}

// normalizeCollectRuleBody applies source text rules in a deterministic,
// idempotent order. It is shared by rule evaluation and the persistence
// boundary so retries cannot write raw text back over cleaned content.
func normalizeCollectRuleBody(text string, rule gdb.Record) string {
	if rule["truncate_intro_fee_enabled"].Bool() {
		text = applyCollectIntroFeeTruncate(text)
	}
	text = applyCollectLineDeletes(text, collectRuleStrings(rule, "delete_lines"))
	text = applyCollectTextDeletes(text, collectRuleStrings(rule, "delete_texts"))
	return applyCollectReplacements(text, collectRuleReplacements(rule))
}

func shouldDropCollectStandaloneCodeCaption(text string, mediaCount int) bool {
	text = strings.TrimSpace(text)
	return mediaCount == 1 && collectStandaloneCodeCaptionRule.MatchString(text)
}

type collectDedupeMaterial struct {
	mediaKey        string
	textHash        string
	imagePHashKey   string
	mediaCount      int
	mediaTotal      int
	imagePHashCount int
	imageTotal      int
}

type collectDedupeSignature struct {
	layer string
	value string
	total int
	count int
}

func (material collectDedupeMaterial) signatures(includePHash bool) []collectDedupeSignature {
	items := make([]collectDedupeSignature, 0, 3)
	if material.textHash != "" {
		items = append(items, collectDedupeSignature{layer: "text_hash", value: material.textHash})
	}
	if material.mediaKey != "" && material.mediaTotal > 0 && material.mediaCount == material.mediaTotal {
		items = append(items, collectDedupeSignature{layer: "media_fingerprint", value: material.mediaKey, total: material.mediaTotal, count: material.mediaCount})
	}
	if includePHash && material.imagePHashKey != "" && material.imageTotal > 0 && material.imagePHashCount == material.imageTotal {
		items = append(items, collectDedupeSignature{layer: "image_phash", value: material.imagePHashKey, total: material.imageTotal, count: material.imagePHashCount})
	}
	return items
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
		fingerprint := strings.ToLower(strings.TrimSpace(item.FileMd5))
		if fingerprint == "" {
			continue
		}
		values = append(values, strings.ToLower(strings.TrimSpace(item.Type))+":md5:"+fingerprint)
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
			if previous := previousCollectContentLine(lines, index); previous >= 0 && collectShortTitleLine(lines[previous]) {
				cutoff = previous
			}
			changed = true
			break
		}
	}
	lines = lines[:cutoff]
	start := 0
	firstContentLine := true
	for start < len(lines) {
		line := strings.TrimSpace(lines[start])
		if line == "" || collectMaterialMetaLineRule.MatchString(line) || collectMaterialCodeLineRule.MatchString(line) || (firstContentLine && isCollectMaterialAlphaNumericCode(line)) {
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

func previousCollectContentLine(lines []string, before int) int {
	for index := before - 1; index >= 0; index-- {
		if strings.TrimSpace(normalizeCollectKeywordText(lines[index])) != "" {
			return index
		}
	}
	return -1
}

func collectShortTitleLine(line string) bool {
	normalized := normalizeCollectKeywordText(strings.TrimSpace(line))
	if strings.ContainsAny(normalized, ":：=;；") {
		return false
	}
	visible := 0
	for _, char := range normalized {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			visible++
		}
	}
	return visible > 0 && visible < 5
}

func isCollectMaterialAlphaNumericCode(line string) bool {
	normalized := normalizeCollectKeywordText(strings.TrimSpace(line))
	if normalized == "" {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, char := range normalized {
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z':
			hasLetter = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case char == '-', char == '_', unicode.IsSpace(char):
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func isCollectIntroFeeSourceMarkLine(line string) bool {
	return strings.Contains(normalizeCollectKeywordText(line), "情诗")
}

func applyCollectIntroFeeSuffix(text, original, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	fields := parseCollectIntroFeeFields(original)
	if suffix == "" {
		return text
	}
	// Suffix replacement is strictly line-local. Do not remove neighbouring
	// lines or truncate the body while rebuilding the fee caption.
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	changed := false
	for index, line := range lines {
		normalized := normalizeCollectKeywordText(line)
		if !collectFeeKeywordPattern.MatchString(normalized) {
			continue
		}
		lineFields := parseCollectIntroFeeFields(line)
		if lineFields.FirstAmount != "" {
			lines[index] = formatCollectIntroFeeFields(lineFields, suffix)
		} else if !strings.HasSuffix(strings.TrimSpace(line), suffix) {
			lines[index] = strings.TrimRightFunc(line, unicode.IsSpace) + " " + suffix
		}
		changed = true
	}
	if changed {
		return strings.Join(lines, "\n")
	}
	amount := fields.FirstAmount
	feeNumberCount := 0
	for _, line := range strings.Split(strings.ReplaceAll(normalizeCollectKeywordText(original), "\r\n", "\n"), "\n") {
		if collectFeeKeywordPattern.MatchString(line) {
			feeNumberCount = len(collectNumberRule.FindAllString(line, -1))
			break
		}
	}
	if feeNumberCount > 1 && fields.ForeignAmount == "" && collectFeeKeywordPattern.MatchString(normalizeCollectKeywordText(text)) {
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		for index, line := range lines {
			if collectFeeKeywordPattern.MatchString(normalizeCollectKeywordText(line)) && !strings.HasSuffix(strings.TrimSpace(line), suffix) {
				lines[index] = strings.TrimRightFunc(line, unicode.IsSpace) + " " + suffix
			}
		}
		return strings.Join(lines, "\n")
	}
	if amount == "" {
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		for index, line := range lines {
			if collectFeeKeywordPattern.MatchString(normalizeCollectKeywordText(line)) && !strings.HasSuffix(strings.TrimSpace(line), suffix) {
				lines[index] = strings.TrimRightFunc(line, unicode.IsSpace) + " " + suffix
			}
		}
		return strings.Join(lines, "\n")
	}
	feeLine := formatCollectIntroFeeFields(fields, suffix)
	if feeLine == "" {
		return text
	}
	lines = strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines)+1)
	removeNextSuffix := false
	for _, line := range lines {
		if removeNextSuffix && isCollectIntroFeeStandaloneSuffix(strings.TrimSpace(line)) {
			removeNextSuffix = false
			continue
		}
		removeNextSuffix = false
		if parseCollectIntroFeeFields(line).FirstAmount != "" {
			removeNextSuffix = true
			continue
		}
		kept = append(kept, line)
	}
	body := strings.TrimSpace(strings.Join(kept, "\n"))
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
		match = collectIntroFeeLooseAmountRule.FindStringSubmatch(normalizeCollectKeywordText(text))
	}
	if len(match) < 2 {
		return ""
	}
	return strings.Join(strings.Fields(strings.TrimSpace(match[1])), " ")
}

func normalizeCollectKeywordText(text string) string {
	text = norm.NFKC.String(text)
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return '0' + (r - '０')
		}
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
