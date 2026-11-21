package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

func (s *sSysPublish) CollectManageChannels(ctx context.Context, in *sysin.CollectManageChannelListInp) ([]*sysin.CollectManageChannelModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.BotCollectManageChannels(ctx, account.TenantId, account.Id, in)
}

func (s *sSysPublish) CollectManageChannelAction(ctx context.Context, in *sysin.CollectManageChannelActionInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.BotCollectManageChannelAction(ctx, account.TenantId, account.Id, in)
}

func (s *sSysPublish) collectManageOperator(ctx context.Context, tenantID, actorID int64) (*sysin.AccountCapabilityModel, error) {
	if tenantID <= 0 {
		return nil, gerror.New("采集租户信息不完整")
	}
	actor, err := s.activeAccountCapability(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if actor.AccountType != sysin.PublishAccountTypeAdmin && (actor.AccountType != sysin.PublishAccountTypeUploader || actor.CollectPushEnabled != 1) {
		return nil, gerror.New("当前账号未开通采集推送权限")
	}
	return actor, nil
}

func collectManageChannelQuery(ctx context.Context, tenantID int64) *gdb.Model {
	return pdao.YoubanPublishCollectAccountChannel.DB().Model(pdao.YoubanPublishCollectAccountChannel.Table()+" ac").Safe().Ctx(ctx).
		InnerJoin(pdao.YoubanPublishCollectSource.Table()+" src", "src.id=ac.source_id AND src.tenant_id=ac.tenant_id AND src.account_id=ac.account_id AND src.deleted_at IS NULL").
		InnerJoin(pdao.YoubanPublishCollectChannel.Table()+" ch", "ch.id=ac.channel_id AND ch.tenant_id=ac.tenant_id AND ch.deleted_at IS NULL").
		Where("ac.tenant_id", tenantID).WhereNull("ac.deleted_at").WhereNot("ac.status", sysin.CollectAccountChannelRemoved)
}

func collectManageRules(query *gdb.Model) *gdb.Model {
	return query.LeftJoin(pdao.YoubanPublishCollectSourceRule.Table()+" sr", "sr.source_id=src.id AND sr.tenant_id=src.tenant_id AND sr.status=1").
		LeftJoin(pdao.YoubanPublishCollectRule.Table()+" r", "r.id=sr.rule_id AND r.tenant_id=src.tenant_id AND r.account_id=src.account_id AND r.deleted_at IS NULL")
}

func (s *sSysPublish) BotCollectManageChannels(ctx context.Context, tenantID, actorID int64, in *sysin.CollectManageChannelListInp) ([]*sysin.CollectManageChannelModel, int, error) {
	if _, err := s.collectManageOperator(ctx, tenantID, actorID); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectManageChannelListInp{}
	}
	in.Page = max(in.Page, 1)
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	in.PerPage = min(in.PerPage, 50)
	query := collectManageRules(collectManageChannelQuery(ctx, tenantID))
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(ch.title LIKE ? OR ch.telegram_chat_id LIKE ? OR ch.username LIKE ?)", like, like, like)
	}
	rows, err := query.Fields("ac.id AS relation_id,ac.account_id,ac.status,ac.approval_status,ch.telegram_chat_id,ch.title,ch.username,ch.invite_link,src.bot_id,r.id AS rule_id,r.name AS rule_name").
		OrderDesc("ac.id").OrderAsc("r.id").Page(in.Page, in.PerPage).All()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取采集频道失败")
	}
	total, err := query.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集频道失败")
	}
	ruleIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if ruleID := row["rule_id"].Int64(); ruleID > 0 {
			ruleIDs = append(ruleIDs, ruleID)
		}
	}
	ruleItems, err := collectRuleItemMap(ctx, uniqueIds(ruleIDs))
	if err != nil {
		return nil, 0, err
	}
	list := make([]*sysin.CollectManageChannelModel, 0, len(rows))
	for _, row := range rows {
		item := &sysin.CollectManageChannelModel{
			RelationId: row["relation_id"].Int64(), RuleId: row["rule_id"].Int64(), RuleName: row["rule_name"].String(),
			ChannelId: row["telegram_chat_id"].String(), ChannelTitle: row["title"].String(),
			ChannelLink: collectChannelPublicLink(row["username"].String(), row["invite_link"].String()),
			Status:      row["status"].String(), ApprovalStatus: row["approval_status"].String(),
		}
		if item.RuleId > 0 {
			var fallback string
			if items := ruleItems[item.RuleId]; items != nil {
				fallback = items.IntroFeeSuffix
			}
			item.Suffix, _, err = s.resolveIntroFeeChannelSuffix(ctx, gdb.Record{
				"tenant_id": g.NewVar(tenantID), "account_id": row["account_id"], "bot_id": row["bot_id"], "source_chat_id": row["telegram_chat_id"],
			}, gdb.Record{"id": row["rule_id"], "intro_fee_suffix": g.NewVar(fallback)})
			if err != nil {
				return nil, 0, err
			}
		}
		list = append(list, item)
	}
	return list, total, nil
}

func (s *sSysPublish) BotCollectManageChannelAction(ctx context.Context, tenantID, actorID int64, in *sysin.CollectManageChannelActionInp) error {
	if err := in.Filter(ctx); err != nil {
		return err
	}
	relationDAO := pdao.YoubanPublishCollectAccountChannel
	cols := relationDAO.Columns()
	var target gdb.Record
	var changed bool
	err := relationDAO.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		actor, err := s.collectManageOperator(ctx, tenantID, actorID)
		if err != nil {
			return err
		}
		query := collectManageChannelQuery(ctx, tenantID).Where("ac.id", in.RelationId)
		if in.Action == "suffix" {
			query = collectManageRules(query).Where("r.id", in.RuleId)
		}
		target, err = query.Fields("src.bot_id,ac.account_id,ac.source_id,ch.telegram_chat_id,ch.title").One()
		if err != nil {
			return err
		}
		if target.IsEmpty() {
			return gerror.New("采集频道或绑定规则不存在")
		}
		change := collectChannelChange{
			TenantID: tenantID, ActorID: actor.AccountId, ActorType: actor.AccountType,
			SourceID: target["source_id"].Int64(), ChannelID: target["telegram_chat_id"].String(), ChannelName: target["title"].String(),
		}
		if in.Action == "suffix" {
			var before string
			before, changed, err = s.saveCollectManageSuffix(ctx, tenantID, target, in)
			if err != nil || !changed {
				return err
			}
			change.Action = "修改频道后缀"
			change.Before, change.After = before, in.Suffix
			return enqueueCollectChannelChange(ctx, change)
		}
		relation, err := tx.Model(relationDAO.Table()).Ctx(ctx).
			Where(cols.Id, in.RelationId).Where(cols.TenantId, tenantID).WhereNull(cols.DeletedAt).LockUpdate().One()
		if err != nil {
			return err
		}
		if relation.IsEmpty() || relation[cols.Status].String() == sysin.CollectAccountChannelRemoved {
			return gerror.New("采集频道不存在或已失效")
		}
		approval := relation[cols.ApprovalStatus].String()
		if in.Action != "disable" && approval != "pending" {
			return gerror.New("频道审核已处理，请刷新")
		}
		if in.Action == "disable" && relation[cols.Status].String() == sysin.CollectAccountChannelDisabled && relation[cols.CollectEnabled].Int() == 0 {
			return nil
		}
		update := g.Map{cols.Status: sysin.CollectAccountChannelDisabled, cols.CollectEnabled: 0, cols.DisabledAt: gtime.Now()}
		change.Action = "禁用频道"
		if in.Action == "reject" {
			update[cols.ApprovalStatus] = "rejected"
			change.Action = "拒绝频道审核"
		}
		if in.Action == "approve" {
			blacklisted, err := collectAccountChannelBlacklisted(ctx, tenantID, relation[cols.AccountId].Int64(), []int64{relation[cols.ChannelId].Int64()})
			if err != nil {
				return err
			}
			if blacklisted {
				return gerror.New("频道在采集黑名单中，请联系管理员处理")
			}
			update[cols.Status] = sysin.CollectAccountChannelEnabled
			update[cols.CollectEnabled] = 1
			update[cols.ApprovalStatus] = "approved"
			update[cols.ApprovedAt] = gtime.Now()
			update[cols.DisabledAt] = gdb.Raw("NULL")
			change.Action = "通过频道审核"
		}
		_, err = tx.Model(relationDAO.Table()).Ctx(ctx).Where(cols.Id, in.RelationId).Data(update).Update()
		if err != nil {
			return err
		}
		changed = true
		return enqueueCollectChannelChange(ctx, change)
	})
	if err != nil || !changed {
		return err
	}
	if in.Action == "suffix" {
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(tenantID, target["account_id"].Int64(), in.RuleId, target["bot_id"].Int64(), canonicalCollectSourceChatID(target["telegram_chat_id"].String())))
	} else {
		s.refreshCollectSourceCache(ctx)
	}
	return nil
}

// BotCollectManageLegacySuffix resolves the legacy administrator menu's rule and channel pair.
func (s *sSysPublish) BotCollectManageLegacySuffix(ctx context.Context, tenantID, actorID, ownerID, ruleID, botID int64, channelID, suffix string) error {
	actor, err := s.collectManageOperator(ctx, tenantID, actorID)
	if err != nil {
		return err
	}
	if actor.AccountType != sysin.PublishAccountTypeAdmin {
		return gerror.New("当前账号无管理权限")
	}
	rows, err := collectManageRules(collectManageChannelQuery(ctx, tenantID)).
		Where("ac.account_id", ownerID).Where("r.id", ruleID).
		Where("src.bot_id", botID).WhereIn("ch.telegram_chat_id", tgChannelCacheLookupIds(channelID)).
		Fields("ac.id").Limit(2).All()
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return gerror.New("无法唯一定位采集频道，请在采集推送中重新选择频道")
	}
	return s.BotCollectManageChannelAction(ctx, tenantID, actorID, &sysin.CollectManageChannelActionInp{
		RelationId: rows[0]["id"].Int64(), RuleId: ruleID, Action: "suffix", Suffix: suffix,
	})
}

func (s *sSysPublish) saveCollectManageSuffix(ctx context.Context, tenantID int64, target gdb.Record, in *sysin.CollectManageChannelActionInp) (string, bool, error) {
	accountID, botID := target["account_id"].Int64(), target["bot_id"].Int64()
	chatID := canonicalCollectSourceChatID(target["telegram_chat_id"].String())
	table := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Safe()
	row, err := table.Where("tenant_id", tenantID).Where("account_id", accountID).
		Where("rule_id", in.RuleId).Where("bot_id", botID).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).
		WhereNull("deleted_at").OrderDesc("id").LockUpdate().One()
	if err != nil {
		return "", false, err
	}
	if !row.IsEmpty() {
		if row["suffix"].String() == in.Suffix {
			return in.Suffix, false, nil
		}
		_, err = table.Where("id", row["id"]).Data(g.Map{"suffix": in.Suffix}).Update()
		return row["suffix"].String(), err == nil, err
	}
	_, err = table.Data(g.Map{
		"tenant_id": tenantID, "account_id": accountID, "rule_id": in.RuleId, "bot_id": botID,
		"channel_id": chatID, "suffix": in.Suffix,
	}).Insert()
	return "", err == nil, err
}
