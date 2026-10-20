package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// A rule-only purge must not switch off the account/channel relation used by
// other rules. Retain a disabled override as a tombstone in the existing table.
func disableCollectRuleChannel(ctx context.Context, tenantID, accountID, ruleID int64, chatID string) error {
	if tenantID <= 0 || accountID <= 0 || ruleID <= 0 || canonicalCollectSourceChatID(chatID) == "" {
		return gerror.New("规则频道下架范围不完整")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Serialize the tombstone upsert without relying on partial unique-index
		// syntax or introducing another migration in the online write path.
		row, err := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).Fields("id").Where("id", ruleID).Where("tenant_id", tenantID).Where("account_id", accountID).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定频道所属规则失败")
		}
		if row.IsEmpty() {
			return gerror.New("频道所属规则不存在")
		}
		model := tx.Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).Where("rule_id", ruleID).Where("bot_id", 0).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).WhereNull("deleted_at")
		existing, err := model.Clone().Fields("id").One()
		if err != nil {
			return err
		}
		if existing.IsEmpty() {
			_, err = tx.Model(introFeeChannelRuleTable).Ctx(ctx).Data(g.Map{"tenant_id": tenantID, "account_id": accountID, "rule_id": ruleID, "bot_id": 0, "channel_id": canonicalCollectSourceChatID(chatID), "suffix": "", "status": 2}).Insert()
		} else {
			_, err = model.Data(g.Map{"status": 2, "suffix": ""}).Update()
		}
		return gerror.Wrap(err, "暂停当前规则的频道采集失败")
	})
}

func collectRuleChannelDisabled(ctx context.Context, tenantID, accountID, ruleID, botID int64, chatID string) (bool, error) {
	if ruleID <= 0 || chatID == "" {
		return false, nil
	}
	count, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).Where("rule_id", ruleID).WhereIn("bot_id", []int64{0, botID}).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).Where("status", 2).WhereNull("deleted_at").Count()
	return count > 0, gerror.Wrap(err, "检查规则频道采集状态失败")
}

func validateCollectRulePurgeProfileScope(ctx context.Context, tenantID, accountID, ruleID int64, profileIDs []int64) error {
	for start := 0; start < len(profileIDs); start += collectSourceDownDeleteBatchSize {
		end := start + collectSourceDownDeleteBatchSize
		if end > len(profileIDs) {
			end = len(profileIDs)
		}
		for _, table := range []string{publishCollectChannelProfileTable, publishCollectDispatchTable} {
			count, err := g.DB().Model(table).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).WhereIn("profile_id", profileIDs[start:end]).WhereGT("rule_id", 0).WhereNot("rule_id", ruleID).Count()
			if err != nil {
				return gerror.Wrap(err, "检查资料跨规则归属失败")
			}
			if count > 0 {
				return gerror.New("部分资料由多个采集规则共用，不能仅按当前规则删除整条资料；请使用明确的来源频道全部规则下架操作")
			}
		}
	}
	return nil
}
