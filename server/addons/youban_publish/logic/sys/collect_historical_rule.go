package sys

import (
	"context"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// collectHistoricalRules resolves the rules recorded by existing dispatches.
// It deliberately does not use the source's current binding: users may change
// that binding after a profile has already been collected.
func (s *sSysPublish) collectHistoricalRules(ctx context.Context, event gdb.Record, tenantID, accountID int64, ruleIDs []int64) (map[int64]gdb.Record, error) {
	resolved := make(map[int64]gdb.Record)
	if len(ruleIDs) == 0 {
		return resolved, nil
	}
	rules, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", tenantID).
		Where("account_id", accountID).
		WhereIn("id", ruleIDs).
		WhereNull("deleted_at").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史分发采集规则失败")
	}
	if err = attachCollectRuleItems(ctx, rules); err != nil {
		return nil, err
	}
	globalRules, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", tenantID).
		Where("account_id", accountID).
		Where("global_enabled", 1).
		WhereNull("deleted_at").
		OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史分发全局文本规则失败")
	}
	for _, rule := range globalRules {
		sanitizeGlobalCollectTextPolicy(rule)
	}
	if err = attachCollectRuleItems(ctx, globalRules); err != nil {
		return nil, err
	}
	mergeGlobalCollectTextPolicy(rules, globalRules)
	for _, rule := range rules {
		rule[collectRuleSourceIDField] = gvar.New(event["source_id"].Int64())
		if suffix, ok, suffixErr := s.resolveIntroFeeChannelSuffix(ctx, event, rule); suffixErr != nil {
			return nil, suffixErr
		} else if ok {
			rule["intro_fee_suffix"] = gvar.New(suffix)
		}
		resolved[rule["id"].Int64()] = rule
	}
	return resolved, nil
}
