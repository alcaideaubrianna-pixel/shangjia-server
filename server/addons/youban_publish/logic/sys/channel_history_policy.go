package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) channelPreservesHistoryMessages(ctx context.Context, tenantID, channelID int64) (bool, error) {
	if tenantID <= 0 || channelID <= 0 {
		return true, nil
	}
	policies, err := s.channelHistoryMessagePolicies(ctx, tenantID, []int64{channelID})
	if err != nil {
		return false, err
	}
	return policies[channelID], nil
}

func (s *sSysPublish) channelHistoryMessagePolicies(ctx context.Context, tenantID int64, channelIDs []int64) (map[int64]bool, error) {
	policies := make(map[int64]bool, len(channelIDs))
	uniqueIDs := make([]int64, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		if _, exists := policies[channelID]; exists {
			continue
		}
		// Missing channel configuration must not authorize a destructive operation.
		policies[channelID] = true
		uniqueIDs = append(uniqueIDs, channelID)
	}
	if tenantID <= 0 || len(uniqueIDs) == 0 {
		return policies, nil
	}
	var rows []struct {
		Id                      int64 `orm:"id"`
		PreserveHistoryMessages int   `orm:"preserve_history_messages"`
	}
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,preserve_history_messages").
		Where("tenant_id", tenantID).
		WhereIn("id", uniqueIDs).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道历史消息设置失败")
	}
	for _, row := range rows {
		policies[row.Id] = row.PreserveHistoryMessages == 1
	}
	return policies, nil
}
