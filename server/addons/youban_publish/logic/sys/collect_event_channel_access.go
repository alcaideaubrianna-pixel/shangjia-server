package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

type collectEventChannelAccess struct {
	ChannelID  int64
	ChannelIDs []int64
	Relation   botCollectChannelRelation
	Allowed    bool
}

// A source can receive multiple channels. Its configured channel_id is never
// evidence of which channel produced this event or which subscription applies.
func resolveCollectEventChannelAccess(ctx context.Context, event gdb.Record, accountID int64) (collectEventChannelAccess, error) {
	var result collectEventChannelAccess
	tenantID, sourceID := event["tenant_id"].Int64(), event["source_id"].Int64()
	chatIDs := tgChannelCacheLookupIds(event["source_chat_id"].String())
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || len(chatIDs) == 0 {
		return result, gerror.New("采集事件缺少实际频道或授权归属")
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	cols := pdao.YoubanPublishCollectChannel.Columns()
	var channels []botCollectChannelCandidate
	err := pdao.YoubanPublishCollectChannel.Ctx(ctx).
		Fields(cols.Id, cols.TelegramChatId, cols.Title).
		Where(cols.TenantId, tenantID).WhereIn(cols.TelegramChatId, chatIDs).
		WhereNull(cols.DeletedAt).OrderDesc(cols.Id).Limit(3).Scan(&channels)
	if !collectOptionalRecordScanSucceeded(err) {
		return result, gerror.Wrap(err, "读取事件实际采集频道失败")
	}
	if len(channels) == 0 {
		return result, gerror.New("事件实际采集频道尚未登记，不能使用采集源配置频道替代")
	}
	if len(channels) > 2 {
		return result, gerror.New("事件实际采集频道存在重复记录，需先确认频道归属")
	}
	ids := make([]int64, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	result.ChannelIDs = ids
	relationCols := pdao.YoubanPublishCollectAccountChannel.Columns()
	err = pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).
		Fields(relationCols.Id, relationCols.ChannelId, relationCols.SourceId,
			relationCols.CollectEnabled, relationCols.Status, relationCols.ApprovalStatus).
		Where(relationCols.TenantId, tenantID).Where(relationCols.AccountId, accountID).
		Where(relationCols.SourceId, sourceID).WhereIn(relationCols.ChannelId, ids).
		WhereNull(relationCols.DeletedAt).
		OrderDesc(relationCols.UpdatedAt).OrderDesc(relationCols.Id).Limit(1).Scan(&result.Relation)
	if !collectOptionalRecordScanSucceeded(err) {
		return result, gerror.Wrap(err, "读取事件实际频道采集授权失败")
	}
	channel := selectBotCollectChannel(channels, event["source_chat_id"].String())
	result.ChannelID = channel.Id
	if result.Relation.Id > 0 {
		// Short and Bot API IDs describe the same channel. Preserve the latest
		// explicit subscription decision rather than preferring an enabled alias.
		result.ChannelID = result.Relation.ChannelId
	}
	result.Allowed = decideCollectChannelAccess(collectChannelAccessPolicyInput{
		TenantID: tenantID, AccountID: accountID, SourceID: sourceID,
		CanonicalChat:  event["source_chat_id"].String(),
		RelationExists: result.Relation.Id > 0, CollectEnabled: result.Relation.CollectEnabled,
		Status: result.Relation.Status, ApprovalStatus: result.Relation.ApprovalStatus,
	}).Allowed
	return result, nil
}

type collectEventBotChannelRelation struct {
	Id        int64  `json:"id"`
	ChannelId int64  `json:"channelId"`
	Status    string `json:"status"`
}

func resolveCollectEventBotChannel(ctx context.Context, tenantID, botID int64, access collectEventChannelAccess) (collectEventBotChannelRelation, error) {
	var relation collectEventBotChannelRelation
	if tenantID <= 0 || botID <= 0 || len(access.ChannelIDs) == 0 || len(access.ChannelIDs) > 2 {
		return relation, gerror.New("采集事件缺少实际BOT频道归属")
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	cols := pdao.YoubanPublishCollectBotChannel.Columns()
	err := pdao.YoubanPublishCollectBotChannel.Ctx(ctx).
		Fields(cols.Id, cols.ChannelId, cols.Status).
		Where(cols.TenantId, tenantID).Where(cols.BotId, botID).
		WhereIn(cols.ChannelId, access.ChannelIDs).WhereNull(cols.DeletedAt).
		OrderDesc(cols.UpdatedAt).OrderDesc(cols.Id).Limit(1).Scan(&relation)
	if !collectOptionalRecordScanSucceeded(err) {
		return relation, gerror.Wrap(err, "读取事件实际BOT频道关系失败")
	}
	return relation, nil
}
