package sys

import (
	"context"

	pdao "hotgo/addons/youban_publish/internal/dao"

	"github.com/gogf/gf/v2/errors/gerror"
)

// The source-zero marker is an account/channel denial. A newly discovered
// source or an enabled alias must not bypass it. Never cache an allow result.
func collectAccountChannelBlacklisted(ctx context.Context, tenantID, accountID int64, channelIDs []int64) (bool, error) {
	if tenantID <= 0 || accountID <= 0 || len(channelIDs) == 0 {
		return false, nil
	}
	cols := pdao.YoubanPublishCollectAccountChannel.Columns()
	row, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).
		Fields(cols.Id).
		Where(cols.TenantId, tenantID).Where(cols.AccountId, accountID).
		WhereIn(cols.ChannelId, channelIDs).Where(cols.SourceId, 0).
		Where(cols.DisabledReason, introFeeChannelBlacklistReason).
		WhereNull(cols.DeletedAt).Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "读取频道黑名单状态失败")
	}
	return !row.IsEmpty(), nil
}
