package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) BotCollectSourceApproval(ctx context.Context, sourceId, tenantId, accountId int64, approved bool) error {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return gerror.New("采集源参数不完整")
	}
	data := g.Map{"collect_enabled": 0, "status": 2}
	if approved {
		data = g.Map{"collect_enabled": 1, "status": 1}
	}
	var source struct {
		ChannelId int64 `json:"channelId"`
		AccountId int64 `json:"accountId"`
	}
	_ = pdao.YoubanPublishCollectSource.Ctx(ctx).Fields("channel_id,account_id").Where("id", sourceId).Where("tenant_id", tenantId).WhereNull("deleted_at").Scan(&source)
	result, err := pdao.YoubanPublishCollectSource.Ctx(ctx).Where("id", sourceId).Where("tenant_id", tenantId).WhereNull("deleted_at").Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集源审批状态失败")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return gerror.New("采集源不存在或已失效")
	}
	if source.ChannelId > 0 && source.AccountId > 0 {
		now := gtime.Now()
		status := sysin.CollectAccountChannelDisabled
		approvalStatus := "rejected"
		if approved {
			status = sysin.CollectAccountChannelEnabled
			approvalStatus = "approved"
		}
		relation := g.Map{"tenant_id": tenantId, "account_id": source.AccountId, "channel_id": source.ChannelId, "source_id": sourceId, "collect_enabled": introFeeBoolToInt(approved), "status": status, "approval_status": approvalStatus, "approved_at": gdbNil(approved), "disabled_at": gdbNil(!approved), "updated_at": now}
		_, _ = pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(relation).OnConflict("tenant_id,account_id,channel_id").OnDuplicate("source_id,collect_enabled,status,approval_status,approved_at,disabled_at,updated_at").Save()
	}
	s.refreshCollectSourceCache(ctx)
	return nil
}

func (s *sSysPublish) BotCollectChannelView(ctx context.Context, relationId, tenantId, operatorAccountId int64) (*sysin.CollectChannelApprovalModel, error) {
	if relationId <= 0 || tenantId <= 0 || operatorAccountId <= 0 {
		return nil, gerror.New("频道审批参数不完整")
	}
	var relation struct {
		Id           int64  `json:"id"`
		SourceId     int64  `json:"sourceId"`
		RuleId       int64  `json:"ruleId"`
		BotId        int64  `json:"botId"`
		AccountId    int64  `json:"accountId"`
		ChannelId    string `json:"channelId"`
		ChannelTitle string `json:"channelTitle"`
		Username     string `json:"username"`
		InviteLink   string `json:"inviteLink"`
	}
	if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).As("ac").
		InnerJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=ac.source_id AND s.tenant_id=ac.tenant_id AND s.deleted_at IS NULL").
		InnerJoin(pdao.YoubanPublishCollectChannel.Table()+" ch", "ch.id=ac.channel_id AND ch.tenant_id=ac.tenant_id AND ch.deleted_at IS NULL").
		LeftJoin(pdao.YoubanPublishCollectSourceRule.Table()+" sr", "sr.source_id=s.id AND sr.tenant_id=s.tenant_id AND sr.status=1").
		Fields("ac.id,ac.source_id,sr.rule_id,s.bot_id,s.account_id,ch.telegram_chat_id AS channel_id,ch.title AS channel_title,ch.username,ch.invite_link").
		Where("ac.id", relationId).Where("ac.tenant_id", tenantId).Scan(&relation); err != nil {
		return nil, gerror.Wrap(err, "读取频道审批记录失败")
	}
	if relation.Id <= 0 {
		return nil, gerror.New("频道审批记录不存在或已失效")
	}
	return &sysin.CollectChannelApprovalModel{SourceId: relation.SourceId, RuleId: relation.RuleId, BotId: relation.BotId, AccountId: relation.AccountId, ChannelId: relation.ChannelId, ChannelTitle: relation.ChannelTitle, ChannelLink: collectChannelPublicLink(relation.Username, relation.InviteLink)}, nil
}

func (s *sSysPublish) BotCollectChannelApproval(ctx context.Context, relationId, tenantId, operatorAccountId int64, approved bool) (*sysin.CollectChannelApprovalModel, error) {
	relation, err := s.BotCollectChannelView(ctx, relationId, tenantId, operatorAccountId)
	if err != nil {
		return nil, err
	}
	status := sysin.CollectAccountChannelDisabled
	approval := "rejected"
	if approved {
		status = sysin.CollectAccountChannelEnabled
		approval = "approved"
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Where("id", relationId).Where("tenant_id", tenantId).Data(g.Map{
		"collect_enabled": introFeeBoolToInt(approved), "status": status, "approval_status": approval,
		"approved_at": gdbNil(approved), "disabled_at": gdbNil(!approved), "updated_at": now,
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新频道审批状态失败")
	}
	s.refreshCollectSourceCache(ctx)
	res := &sysin.CollectChannelApprovalModel{SourceId: relation.SourceId, RuleId: relation.RuleId, BotId: relation.BotId, AccountId: relation.AccountId, ChannelId: relation.ChannelId, ChannelTitle: relation.ChannelTitle, ChannelLink: relation.ChannelLink}
	if approved && relation.RuleId > 0 {
		if rule, ruleErr := s.BotCollectRuleView(ctx, relation.RuleId, tenantId, relation.AccountId); ruleErr == nil && rule != nil {
			res.DefaultSuffix = strings.TrimSpace(rule.IntroFeeSuffix)
		}
	}
	return res, nil
}
