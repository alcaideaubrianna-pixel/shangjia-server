package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

type collectMaterialPreflightResult struct {
	Rules  []gdb.Record
	Stage  string
	Reason string
}

// preflightCollectMaterialGroup only performs inexpensive rule matching.
// Content dedupe must run after each rule has produced its final text.
func (s *sSysPublish) preflightCollectMaterialGroup(ctx context.Context, event gdb.Record, rules []gdb.Record) (*collectMaterialPreflightResult, error) {
	result := &collectMaterialPreflightResult{Stage: "precheck"}
	// Auto-delete keywords are terminal filters for both Bot and account
	// collection. Apply them before rule matching, media canonicalization and
	// fingerprint dedupe so rejected messages do not consume those resources.
	if tenantID := event["tenant_id"].Int64(); tenantID > 0 {
		if conf, err := (&sSysConfig{}).AutoDeleteConfigForTenant(ctx, tenantID); err != nil {
			return nil, err
		} else if conf != nil && conf.AutoDeleteConfig != nil && conf.AutoDeleteConfig.Enabled == 1 {
			if keyword := matchedAutoDeleteKeyword(event["raw_text"].String(), conf.AutoDeleteConfig.Keywords); keyword != "" {
				result.Reason = "删除关键字忽略:" + keyword
				return result, nil
			}
		}
	}
	if len(rules) == 0 {
		result.Reason = "未命中可用规则"
		return result, nil
	}
	candidateRules, reasons := s.precheckCollectEventRules(event, rules)
	if len(candidateRules) == 0 {
		result.Reason = "未命中规则或被屏蔽"
		if len(reasons) > 0 {
			result.Reason = strings.Join(uniqueStrings(reasons), "；")
		}
		return result, nil
	}
	result.Rules = candidateRules
	return result, nil
}
