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
func (s *sSysPublish) preflightCollectMaterialGroup(_ context.Context, event gdb.Record, rules []gdb.Record) (*collectMaterialPreflightResult, error) {
	result := &collectMaterialPreflightResult{Stage: "precheck"}
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
