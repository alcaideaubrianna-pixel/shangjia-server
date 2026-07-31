package sys

import (
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

type collectRulePrecheckResult struct {
	Matched   bool
	Reason    string
	MatchJSON string
}

func (s *sSysPublish) precheckCollectEventRules(event gdb.Record, rules []gdb.Record) ([]gdb.Record, []string) {
	candidates := make([]gdb.Record, 0, len(rules))
	reasons := make([]string, 0, len(rules))
	for _, rule := range rules {
		result := precheckCollectRule(event, rule)
		if result.Matched {
			candidates = append(candidates, rule)
			continue
		}
		if strings.TrimSpace(result.Reason) != "" {
			reasons = append(reasons, strings.TrimSpace(result.Reason))
		}
	}
	return candidates, reasons
}

func precheckCollectRule(event gdb.Record, rule gdb.Record) collectRulePrecheckResult {
	return precheckCollectRuleText(event["raw_text"].String(), event["media_count"].Int(), rule)
}

func precheckCollectRuleText(rawText string, mediaCount int, rule gdb.Record) collectRulePrecheckResult {
	rawText = strings.TrimSpace(rawText)
	if rule["block_plain_text"].Int() == 1 && mediaCount == 0 {
		return skippedCollectRulePrecheck("消息组没有媒体")
	}
	if rule["block_link"].Int() == 1 && collectLinkPattern.MatchString(rawText) {
		return skippedCollectRulePrecheck("链接")
	}
	if rule["block_username"].Int() == 1 && collectUsernamePattern.MatchString(rawText) {
		return skippedCollectRulePrecheck("用户名")
	}
	if blockedText := matchCollectTerms(rawText, collectRuleStrings(rule, "blocked_texts")); blockedText != "" {
		return skippedCollectRulePrecheck("屏蔽文本:" + blockedText)
	}
	if rawText == "" && mediaCount > 0 {
		return collectRulePrecheckResult{
			Matched:   true,
			MatchJSON: collectMatchJSON(nil, nil),
		}
	}
	matchedKeywords := []string(nil)
	matchedTags := []string(nil)
	if rule["full_match_enabled"].Int() != 1 {
		keywords := collectRuleStrings(rule, "keywords")
		matchedKeywords = matchedCollectTerms(rawText, keywords)
		if len(keywords) > 0 && len(matchedKeywords) == 0 {
			return skippedCollectRulePrecheck("未命中关键词")
		}
		tags := collectRuleStrings(rule, "tags")
		matchedTags = matchedCollectTags(rawText, tags)
		if len(tags) > 0 && len(matchedTags) == 0 {
			return skippedCollectRulePrecheck("未命中标签")
		}
	}
	return collectRulePrecheckResult{
		Matched:   true,
		MatchJSON: collectMatchJSON(matchedKeywords, matchedTags),
	}
}

func skippedCollectRulePrecheck(reason string) collectRulePrecheckResult {
	return collectRulePrecheckResult{
		Matched:   false,
		Reason:    strings.TrimSpace(reason),
		MatchJSON: collectMatchJSON(nil, nil),
	}
}
