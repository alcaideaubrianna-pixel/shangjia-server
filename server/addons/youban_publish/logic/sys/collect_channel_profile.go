package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishCollectChannelProfileTable = "hg_youban_publish_collect_channel_profile"

// indexCollectChannelProfile maintains the compact channel/profile index used
// by channel management. Its unique key makes retries idempotent.
func indexCollectChannelProfile(ctx context.Context, tenantID, accountID, sourceID, ruleID, profileID int64, sourceChatID string) error {
	channelID := canonicalCollectSourceChatID(sourceChatID)
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 || profileID <= 0 || channelID == "" {
		return nil
	}
	_, err := g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx).Data(g.Map{
		"tenant_id": tenantID, "account_id": accountID, "source_id": sourceID,
		"rule_id": ruleID, "channel_id": channelID, "profile_id": profileID,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).OnConflict("tenant_id,account_id,rule_id,channel_id,profile_id").OnDuplicate("source_id,updated_at").Save()
	return gerror.Wrap(err, "更新采集频道资料计数失败")
}

func collectChannelProfileIDs(ctx context.Context, tenantID, accountID, ruleID int64, channelID string) ([]int64, error) {
	channelID = canonicalCollectSourceChatID(channelID)
	var rows []struct {
		ProfileID int64 `json:"profileId"`
	}
	err := g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx).
		Fields("DISTINCT profile_id").Where("tenant_id", tenantID).Where("account_id", accountID).
		Where("rule_id", ruleID).WhereIn("channel_id", tgChannelCacheLookupIds(channelID)).
		WhereNull("deleted_at").WhereGT("profile_id", 0).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道关联资料失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ProfileID)
	}
	return uniqueIds(ids), nil
}

func normalizeChannelProfileID(value string) string {
	return strings.TrimSpace(canonicalCollectSourceChatID(value))
}
