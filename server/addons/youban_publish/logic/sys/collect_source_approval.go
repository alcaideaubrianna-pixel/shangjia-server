package sys

import (
	"context"
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

func (s *sSysPublish) BotCollectChannelApproval(ctx context.Context, relationId, tenantId, operatorAccountId int64, approved bool) (int64, error) {
	if relationId <= 0 || tenantId <= 0 || operatorAccountId <= 0 {
		return 0, gerror.New("频道审批参数不完整")
	}
	var relation struct {
		Id       int64 `json:"id"`
		SourceId int64 `json:"sourceId"`
	}
	if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Fields("id,source_id").Where("id", relationId).Where("tenant_id", tenantId).Scan(&relation); err != nil {
		return 0, gerror.Wrap(err, "读取频道审批记录失败")
	}
	if relation.Id <= 0 {
		return 0, gerror.New("频道审批记录不存在或已失效")
	}
	status := sysin.CollectAccountChannelDisabled
	approval := "rejected"
	if approved {
		status = sysin.CollectAccountChannelEnabled
		approval = "approved"
	}
	now := gtime.Now()
	_, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Where("id", relation.Id).Data(g.Map{
		"collect_enabled": introFeeBoolToInt(approved), "status": status, "approval_status": approval,
		"approved_at": gdbNil(approved), "disabled_at": gdbNil(!approved), "updated_at": now,
	}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "更新频道审批状态失败")
	}
	s.refreshCollectSourceCache(ctx)
	return relation.SourceId, nil
}
