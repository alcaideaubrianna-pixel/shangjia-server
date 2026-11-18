package sys

import (
	"context"
	"database/sql"

	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// A source-zero marker applies only to this account and canonical channel.
// Ordinary source-zero disabled rows are not blacklist markers.
const introFeeChannelBlacklistReason = "intro_fee_channel_blacklist"

type introFeeChannelBlacklistRelation struct {
	Id             int64       `orm:"id"`
	ChannelId      int64       `orm:"channel_id"`
	SourceId       int64       `orm:"source_id"`
	CollectEnabled int         `orm:"collect_enabled"`
	Status         string      `orm:"status"`
	ApprovalStatus string      `orm:"approval_status"`
	DisabledReason string      `orm:"disabled_reason"`
	DeletedAt      *gtime.Time `orm:"deleted_at"`
}

func introFeeChannelBlacklistCanClaim(row introFeeChannelBlacklistRelation) bool {
	if row.DisabledReason != "" || row.DeletedAt != nil {
		return false
	}
	if row.SourceId == 0 {
		return botCollectChannelRelationEnabled(row.Status) ||
			row.CollectEnabled == 0 && row.Status == sysin.CollectAccountChannelDisabled && row.ApprovalStatus == "rejected"
	}
	return botCollectChannelRelationEnabled(row.Status)
}

func introFeeChannelBlacklistAffected(result sql.Result, err error, expected int64) (int64, error) {
	if err != nil {
		return 0, gerror.Wrap(err, "保存频道黑名单失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "读取频道黑名单变更数量失败")
	}
	if expected >= 0 && affected != expected {
		return 0, gerror.New("频道状态已变化，请刷新后重试")
	}
	return affected, nil
}

// Both APIs use the same account/channel transaction. Never pause the shared
// source: its configured chat can differ from the actual collected channel.
func (s *sSysPublish) setIntroFeeChannelBlacklist(ctx context.Context, tenantID, accountID, ruleID, botID int64, chatID string, blacklisted int) error {
	chatID = canonicalCollectSourceChatID(chatID)
	if tenantID <= 0 || accountID <= 0 || botID < 0 || ruleID < 0 || chatID == "" {
		return gerror.New("频道黑名单缺少有效账户或频道范围")
	}
	if blacklisted != 0 && blacklisted != 1 {
		return gerror.New("黑名单状态只能为0或1")
	}
	aliases := tgChannelCacheLookupIds(chatID)
	var markerChanges, relationChanges, legacyChanges int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// A saved rule row is already channel ownership evidence. Otherwise
		// resolve only bounded sources, using the existing BOT/channel resolver.
		owned := false
		if ruleID > 0 {
			count, err := tx.Model(introFeeChannelRuleTable).Ctx(ctx).
				Where("tenant_id", tenantID).WhereIn("account_id", []int64{accountID, 0}).
				Where("rule_id", ruleID).WhereIn("channel_id", aliases).
				WhereIn("bot_id", []int64{0, botID}).WhereNull("deleted_at").Count()
			if err != nil {
				return gerror.Wrap(err, "读取频道规则归属失败")
			}
			owned = count > 0
		}
		if !owned {
			sc := pdao.YoubanPublishCollectSource.Columns()
			mod := pdao.YoubanPublishCollectSource.Ctx(ctx).
				Fields(sc.Id, sc.SourceChatId, sc.BotId).
				Where(sc.TenantId, tenantID).Where(sc.AccountId, accountID)
			if ruleID > 0 {
				mod = mod.Where("EXISTS (SELECT 1 FROM "+pdao.YoubanPublishCollectSourceRule.Table()+
					" sr WHERE sr.source_id="+publishCollectSourceTable+".id AND sr.tenant_id=? AND sr.rule_id=?)", tenantID, ruleID)
			}
			if botID > 0 {
				mod = mod.Where(sc.BotId, botID)
			}
			var sources []introFeeChannelSourceScope
			if err := mod.Limit(introFeeChannelSourceScopeLimit + 1).Scan(&sources); err != nil {
				return gerror.Wrap(err, "读取频道采集源归属失败")
			}
			bindings, err := resolveIntroFeeChannelSourceBindings(ctx, tenantID, accountID, botID, chatID, sources)
			if err != nil {
				return err
			}
			for _, matches := range bindings {
				owned = owned || matches
			}
		}
		if !owned {
			return gerror.New("频道未绑定当前账户或规则的采集源")
		}

		ch := pdao.YoubanPublishCollectChannel.Columns()
		var channels []struct{ Id int64 }
		if err := tx.Model(pdao.YoubanPublishCollectChannel.Table()).Ctx(ctx).
			Fields(ch.Id).Where(ch.TenantId, tenantID).WhereIn(ch.TelegramChatId, aliases).
			OrderAsc(ch.Id).LockUpdate().Scan(&channels); err != nil {
			return gerror.Wrap(err, "锁定黑名单频道失败")
		}
		if len(channels) == 0 {
			return gerror.New("频道记录不存在，请刷新频道列表后重试")
		}
		channelIDs := make([]int64, 0, len(channels))
		for _, channel := range channels {
			channelIDs = append(channelIDs, channel.Id)
		}
		ac := pdao.YoubanPublishCollectAccountChannel.Columns()
		base := func() *gdb.Model {
			return tx.Model(pdao.YoubanPublishCollectAccountChannel.Table()).Ctx(ctx).
				Where(ac.TenantId, tenantID).Where(ac.AccountId, accountID).WhereIn(ac.ChannelId, channelIDs)
		}
		var relations []introFeeChannelBlacklistRelation
		if err := base().Unscoped().OrderAsc(ac.Id).LockUpdate().Scan(&relations); err != nil {
			return gerror.Wrap(err, "锁定频道账户关系失败")
		}
		markers := make(map[int64]bool, len(channels))
		for _, row := range relations {
			if row.SourceId == 0 {
				markers[row.ChannelId] = true
				// Never erase a purge tombstone, pending approval or somebody
				// else's disable reason to manufacture a successful blacklist.
				if row.DeletedAt != nil || row.DisabledReason != "" && row.DisabledReason != introFeeChannelBlacklistReason ||
					blacklisted == 1 && row.DisabledReason == "" && !introFeeChannelBlacklistCanClaim(row) {
					return gerror.New("频道存在其他禁用或待审核状态，不能覆盖")
				}
			}
		}
		now := gtime.Now()
		for _, row := range relations {
			if row.DeletedAt != nil {
				continue
			}
			var data pdo.YoubanPublishCollectAccountChannel
			if blacklisted == 1 {
				if row.DisabledReason != introFeeChannelBlacklistReason && !introFeeChannelBlacklistCanClaim(row) {
					continue
				}
				data = pdo.YoubanPublishCollectAccountChannel{
					CollectEnabled: 0, Status: sysin.CollectAccountChannelDisabled,
					DisabledReason: introFeeChannelBlacklistReason, DisabledAt: now,
				}
				if row.SourceId == 0 {
					data.ApprovalStatus = "rejected"
				}
			} else {
				if row.DisabledReason != introFeeChannelBlacklistReason {
					continue
				}
				// Removing the marker must not create a generic authorization
				// that overrides another source/rule's pending or purge state.
				if row.SourceId == 0 {
					data.DisabledReason = ""
				} else {
					if row.CollectEnabled != 0 || row.Status != sysin.CollectAccountChannelDisabled || row.ApprovalStatus != "approved" {
						return gerror.New("黑名单关系状态已变化，不能自动恢复")
					}
					data = pdo.YoubanPublishCollectAccountChannel{
						CollectEnabled: 1, Status: sysin.CollectAccountChannelEnabled,
						DisabledReason: "", ApprovedAt: now,
					}
				}
			}
			result, err := base().Where(ac.Id, row.Id).Where(ac.DisabledReason, row.DisabledReason).Data(data).Update()
			n, err := introFeeChannelBlacklistAffected(result, err, 1)
			if err != nil {
				return err
			}
			if row.SourceId == 0 {
				markerChanges += n
			} else {
				relationChanges += n
			}
		}
		if blacklisted == 1 {
			for _, channelID := range channelIDs {
				if markers[channelID] {
					continue
				}
				result, err := tx.Model(pdao.YoubanPublishCollectAccountChannel.Table()).Ctx(ctx).
					Data(pdo.YoubanPublishCollectAccountChannel{
						TenantId: tenantID, AccountId: accountID, ChannelId: channelID, SourceId: 0,
						CollectEnabled: 0, Status: sysin.CollectAccountChannelDisabled, ApprovalStatus: "rejected",
						DisabledReason: introFeeChannelBlacklistReason, DisabledAt: now,
					}).Insert()
				n, err := introFeeChannelBlacklistAffected(result, err, 1)
				if err != nil {
					return err
				}
				markerChanges += n
			}
		}
		// Compatibility only for direct sources of this exact channel. No
		// BOT root/configured-chat update, nor shared-source status change.
		sc := pdao.YoubanPublishCollectSource.Columns()
		result, err := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
			Where(sc.TenantId, tenantID).Where(sc.AccountId, accountID).
			WhereIn(sc.SourceChatId, aliases).WhereNot(sc.ChannelBlacklisted, blacklisted).
			Data(pdo.YoubanPublishCollectSource{ChannelBlacklisted: blacklisted}).Update()
		legacyChanges, err = introFeeChannelBlacklistAffected(result, err, -1)
		return err
	})
	if err != nil {
		return err
	}
	g.Log().Info(ctx, "频道黑名单保存完成", g.Map{
		"event": "collect.channel.blacklist_saved", "outcome": "success",
		"tenantId": tenantID, "accountId": accountID, "ruleId": ruleID,
		"botId": botID, "canonicalChatId": chatID, "blacklisted": blacklisted,
		"markerAffected": markerChanges, "relationAffected": relationChanges, "legacyAffected": legacyChanges,
	})
	return nil
}
