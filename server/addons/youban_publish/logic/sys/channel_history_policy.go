package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) channelPreservesHistoryMessages(ctx context.Context, tenantID, channelID int64) (bool, error) {
	if tenantID <= 0 || channelID <= 0 {
		return false, nil
	}
	value, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("preserve_history_messages").
		Where("tenant_id", tenantID).
		Where("id", channelID).
		Value()
	if err != nil {
		return false, gerror.Wrap(err, "读取频道历史消息设置失败")
	}
	return value.Int() == 1, nil
}
