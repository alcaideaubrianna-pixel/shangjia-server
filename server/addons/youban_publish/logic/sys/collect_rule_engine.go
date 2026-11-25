package sys

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	introfee "hotgo/addons/youban_publish/logic/introfee"
)

const collectFeeKeywordExpression = `(?:介绍费|介绍费用|推荐费|牵线费|居间费|对接费|感谢费|辛苦费|包含费|服务费|中介费|中介服务费|大米)`

var (
	collectLinkPattern               = regexp.MustCompile(`(?i)(https?://|t\.me/|telegram\.me/)`)
	collectUsernamePattern           = regexp.MustCompile(`@`)
	collectStandaloneCodeCaptionRule = regexp.MustCompile(`^[A-Za-z]{1,4}\d{3,6}$`)
	collectMaterialMetaLineRule      = regexp.MustCompile(`^\s*(?:标题|昵称|编号|编码|同行)\s*(?:[:：=].*)?\s*$`)
	collectMaterialCodeLineRule      = regexp.MustCompile(`^\s*(?:[A-Za-z]{1,5}[-_ ]?[0-9０-９]{2,10}|[\p{Han}]{1,8}[-_ ]?[A-Za-z0-9０-９]{2,8})(?:[^A-Za-z0-9０-９]|$)`)
	collectFeeKeywordPattern         = regexp.MustCompile(collectFeeKeywordExpression)
	collectCriticalProfileLineRules  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:年龄|年齡|age)[^0-9０-９\n]{0,12}[0-9０-９]`),
		regexp.MustCompile(`(?i)(?:身高|height)[^0-9０-９\n]{0,12}[0-9０-９]`),
		regexp.MustCompile(`(?i)(?:体重|體重|weight)[^0-9０-９\n]{0,12}[0-9０-９]`),
	}
)

// Telegram forwards frequently replace the fee separator with full-width or
// decorative punctuation. These characters are separators, not part of the
// fee label or amount.
type collectRuleDecision struct {
	Matched   bool
	Skipped   bool
	Reason    string
	Text      string
	MatchJSON string
}

type collectIntroFeeFields = introfee.Fields

func parseCollectIntroFeeFields(text string) collectIntroFeeFields {
	return introfee.ParseFields(text)
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
	return &collectRuleDecision{
		Matched: true, Text: renderCollectRuleText(event, content, rule), MatchJSON: precheck.MatchJSON,
	}
}

// renderCollectRuleText is the single rule-rendering entry point. Persistence
// and review paths use the same normalizer with their current text snapshot.
func renderCollectRuleText(event gdb.Record, content *collectContentResult, rule gdb.Record) string {
	rawText := authoritativeCollectRawText(event["raw_text"].String())
	if content != nil {
		rawText = authoritativeCollectRawText(content.RawText)
	}
	if strings.TrimSpace(rawText) == "" {
		return ""
	}
	renderEvent := make(gdb.Record, len(event)+1)
	for key, value := range event {
		renderEvent[key] = value
	}
	renderEvent["raw_text"] = gvar.New(rawText)
	return (&sSysPublish{}).normalizeCollectMaterialText(context.Background(), renderEvent, rule, rawText)
}

func normalizeCollectRuleBodyWithoutTruncate(text string, rule gdb.Record) string {
	return applyCollectFeeScopedRules(text, rule, false)
}

// normalizeCollectRuleBody applies source text rules in a deterministic,
// idempotent order. It is shared by rule evaluation and the persistence
// boundary so retries cannot write raw text back over cleaned content.
func normalizeCollectRuleBody(text string, rule gdb.Record) string {
	truncateEnabled := rule["truncate_intro_fee_enabled"].Bool()
	if truncateEnabled {
		text = applyCollectLeadingMaterialTitleCleanup(text)
	}
	truncate := truncateEnabled && strings.TrimSpace(rule["intro_fee_suffix"].String()) == ""
	return applyCollectFeeScopedRules(text, rule, truncate)
}

// normalizeCollectTextInOrder is the single collection text pipeline.
// Fee data is captured from the original text before any destructive rule runs.
func normalizeCollectTextInOrder(rawText string, rule gdb.Record, mediaCount int) string {
	lines := strings.Split(strings.ReplaceAll(rawText, "\r\n", "\n"), "\n")
	body := make([]string, 0, len(lines))
	truncateIntroFee := rule["truncate_intro_fee_enabled"].Bool()
	introFeeSuffix := strings.TrimSpace(rule["intro_fee_suffix"].String())
	for _, line := range lines {
		if !introfee.IsFeeLine(line) {
			body = append(body, line)
			continue
		}
		if truncateIntroFee && introFeeSuffix == "" {
			continue
		}
		if introFeeSuffix != "" {
			line = applyCollectIntroFeeSuffix(line, rawText, introFeeSuffix)
		}
		body = append(body, line)
	}
	text := strings.Join(body, "\n")
	if truncateIntroFee {
		text = applyCollectLeadingMaterialTitleCleanup(text)
	}
	text = applyCollectLineDeletes(text, collectRuleStrings(rule, "delete_lines"))
	text = applyCollectTextDeletes(text, collectRuleStrings(rule, "delete_texts"))
	text = applyCollectReplacements(text, collectRuleReplacements(rule))
	if shouldDropCollectStandaloneCodeCaption(text, mediaCount) {
		return ""
	}
	if header := strings.TrimSpace(rule["header_markdown"].String()); rule["header_enabled"].Int() == 1 && header != "" {
		text = header + "\n\n" + text
	}
	if footer := strings.TrimSpace(rule["footer_markdown"].String()); rule["footer_enabled"].Int() == 1 && footer != "" {
		text = strings.TrimSpace(text + "\n" + footer)
	}
	return text
}

// Fee lines and explicit profile numbers are content, not source title codes.
func isCollectLeadingMaterialTitle(line string) bool {
	normalized := normalizeCollectKeywordText(line)
	if introfee.IsFeeLine(normalized) || strings.HasPrefix(strings.TrimSpace(line), "编号") {
		return false
	}
	return collectMaterialMetaLineRule.MatchString(line) || collectMaterialCodeLineRule.MatchString(line) || isCollectMaterialAlphaNumericCode(line)
}

func applyCollectLeadingMaterialTitleCleanup(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	limit := min(3, len(lines))
	result := make([]string, 0, len(lines))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index < limit && isCollectLeadingMaterialTitle(trimmed) {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// applyCollectFeeScopedRules confines every destructive rule to the fee line
// and its immediate neighbours. Lines outside these windows are immutable.
func applyCollectFeeScopedRules(text string, rule gdb.Record, truncate bool) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	allowed := make([]bool, len(lines))
	for index, line := range lines {
		if !introfee.IsFeeLine(line) {
			continue
		}
		for candidate := index - 1; candidate <= index+1; candidate++ {
			if candidate >= 0 && candidate < len(lines) {
				allowed[candidate] = true
			}
		}
	}
	result := make([]string, 0, len(lines))
	for index := 0; index < len(lines); {
		if !allowed[index] {
			result = append(result, lines[index])
			index++
			continue
		}
		end := index + 1
		for end < len(lines) && allowed[end] {
			end++
		}
		window := strings.Join(lines[index:end], "\n")
		if truncate {
			window = applyCollectIntroFeeTruncateAt(window, index)
		}
		window = applyCollectLineDeletes(window, collectRuleStrings(rule, "delete_lines"))
		window = applyCollectTextDeletes(window, collectRuleStrings(rule, "delete_texts"))
		window = applyCollectReplacements(window, collectRuleReplacements(rule))
		if window != "" {
			result = append(result, strings.Split(window, "\n")...)
		}
		index = end
	}
	return strings.Join(result, "\n")
}

// preserveCollectNonFeeLines is the final persistence invariant. All source
// lines outside the fee line and its immediate neighbours must survive.
func preserveCollectNonFeeLines(original, processed string) string {
	return preserveCollectNonFeeLinesWithFilter(original, processed, nil)
}

func preserveCollectNonFeeLinesWithFilter(original, processed string, shouldRestore func(string) bool) string {
	originalLines := strings.Split(strings.ReplaceAll(original, "\r\n", "\n"), "\n")
	protected := make([]bool, len(originalLines))
	for index := range protected {
		protected[index] = true
	}
	for index := 0; index < min(3, len(originalLines)); index++ {
		trimmed := strings.TrimSpace(originalLines[index])
		if isCollectLeadingMaterialTitle(trimmed) {
			protected[index] = false
		}
	}
	for index, line := range originalLines {
		if introfee.IsFeeLine(line) {
			for candidate := index - 1; candidate <= index+1; candidate++ {
				if candidate >= 0 && candidate < len(protected) {
					protected[candidate] = false
				}
			}
		}
	}
	missing := make([]string, 0)
	remaining := processed
	for index, line := range originalLines {
		if !protected[index] || strings.TrimSpace(line) == "" {
			continue
		}
		if shouldRestore != nil && !shouldRestore(line) {
			continue
		}
		if position := strings.Index(remaining, line); position >= 0 {
			remaining = remaining[position+len(line):]
			continue
		}
		missing = append(missing, line)
	}
	if len(missing) == 0 {
		return processed
	}
	processedLines := strings.Split(strings.ReplaceAll(processed, "\r\n", "\n"), "\n")
	if processed == "" {
		processedLines = nil
	}
	insertAt := len(processedLines)
	for index, line := range processedLines {
		if introfee.IsFeeLine(line) {
			insertAt = index
			break
		}
	}
	result := make([]string, 0, len(processedLines)+len(missing))
	result = append(result, processedLines[:insertAt]...)
	result = append(result, missing...)
	result = append(result, processedLines[insertAt:]...)
	return strings.Join(result, "\n")
}

// Kept as a compatibility alias for callers and historical tests.
func preserveCollectCriticalProfileLines(original, processed string) string {
	return preserveCollectNonFeeLines(original, processed)
}

func shouldDropCollectStandaloneCodeCaption(text string, mediaCount int) bool {
	text = strings.TrimSpace(text)
	return mediaCount == 1 && collectStandaloneCodeCaptionRule.MatchString(text)
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
	return applyCollectIntroFeeTruncateAt(text, 0)
}

func applyCollectIntroFeeTruncateAt(text string, lineOffset int) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	changed := false
	cutoff := len(lines)
	for index, line := range lines {
		// Match against a normalized copy only; preserve the original text,
		// emoji, spacing, and formatting in all retained lines.
		if collectIntroFeeAmount(line) != "" || introfee.IsFeeLine(line) {
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
		if line == "" || collectMaterialMetaLineRule.MatchString(line) || (lineOffset+start < 3 && collectMaterialCodeLineRule.MatchString(line)) || (lineOffset+start < 3 && firstContentLine && isCollectMaterialAlphaNumericCode(line)) {
			start++
			changed = true
			continue
		}
		firstContentLine = false
		break
	}
	kept := make([]string, 0, len(lines)-start)
	for index, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if collectMaterialMetaLineRule.MatchString(trimmed) || (lineOffset+start+index < 3 && collectMaterialCodeLineRule.MatchString(trimmed)) || isCollectIntroFeeSourceMarkLine(trimmed) {
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
	return introfee.IsShortTitle(line)
}

func isCollectMaterialAlphaNumericCode(line string) bool {
	normalized := normalizeCollectKeywordText(strings.TrimSpace(line))
	if normalized == "" {
		return false
	}
	hasLetter := false
	hasDigit := false
	visible := 0
	hasHan := false
	for _, char := range normalized {
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z':
			hasLetter = true
			visible++
		case unicode.Is(unicode.Han, char):
			hasLetter = true
			hasHan = true
			visible++
		case char >= '0' && char <= '9':
			hasDigit = true
			visible++
		case char == '-', char == '_', unicode.IsSpace(char):
		default:
			return false
		}
	}
	return hasLetter && hasDigit && (!hasHan || visible <= 8)
}

func isCollectIntroFeeSourceMarkLine(line string) bool {
	return strings.Contains(normalizeCollectKeywordText(line), "情诗")
}

func applyCollectIntroFeeSuffix(text, original, suffix string) string {
	limit := 6
	if configured, err := strconv.Atoi(strings.TrimSpace(os.Getenv("YOUBAN_INTRO_FEE_NEIGHBOR_MAX_CHARS"))); err == nil && configured > 0 {
		limit = configured
	}
	return introfee.ProcessSuffix(text, suffix, limit, func(clean string) bool {
		for _, rule := range collectCriticalProfileLineRules {
			if rule.MatchString(clean) {
				return true
			}
		}
		return false
	})
}

func isCollectIntroFeeStandaloneSuffix(text string) bool {
	text = strings.TrimSpace(text)
	if !introfee.IsStandaloneSuffix(text) {
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
	return introfee.Parse(text).Amount
}

func normalizeCollectKeywordText(text string) string {
	return introfee.Normalize(text)
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
