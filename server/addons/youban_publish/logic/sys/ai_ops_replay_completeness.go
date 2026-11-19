package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// validateAIOpsReplayCompleteness fences explicit channel-range repairs against
// receiver-split shared facts. It never synthesizes updates with another Bot's
// file_id or expands a replay beyond the approved interval.
func validateAIOpsReplayCompleteness(ctx context.Context, tenantID, accountID, sourceID int64, chatID string, startID, endID int64, candidateIDs []int64) error {
	if startID <= 0 || endID < startID || endID-startID >= 10000 {
		return gerror.New("重放完整性检查需要最多10000个消息号的明确区间")
	}
	ctx, cancel := context.WithTimeout(ctx, aiOpsRepairResultTimeout)
	defer cancel()
	// The unique (chat_id,message_id) index bounds this read. Do not filter
	// tenant_id/received_bot_id: those fields describe one retained receiver.
	rows, err := g.DB().Model(telegramBotMessageSourceTable).Ctx(ctx).
		Fields("message_id").
		WhereIn("chat_id", tgChannelCacheLookupIds(chatID)).
		WhereBetween("message_id", startID, endID).
		Where("tg_file_id <> ''").WhereNull("deleted_at").
		Limit(int(endID-startID+1) * 2).All()
	if err != nil {
		return gerror.Wrap(err, "核对重放区间共享消息事实失败")
	}
	factIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		factIDs = append(factIDs, row["message_id"].Int64())
	}
	missing := collectReplayMissingMessageIDs(factIDs, candidateIDs)
	if len(missing) == 0 {
		return nil
	}
	g.Log().Warning(ctx, "采集重放区间缺少共享消息事实，拒绝部分重放", g.Map{
		"event": "collect.replay.incomplete", "tenantId": tenantID,
		"accountId": accountID, "sourceId": sourceID, "sourceChatId": chatID,
		"startMessageId": startID, "endMessageId": endID,
		"candidateCount": len(candidateIDs), "missingCount": len(missing),
		"missingMessageIds": missing[:min(len(missing), 50)],
	})
	return gerror.Newf("重放区间有%d条媒体消息未进入目标采集源，首条消息%d；请先补全共享消息事实，禁止部分恢复后发布", len(missing), missing[0])
}
