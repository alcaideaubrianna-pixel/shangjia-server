package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func collectSourceGroupKey(event gdb.Record) string {
	if event.IsEmpty() {
		return ""
	}
	chatID := canonicalCollectSourceChatID(event["source_chat_id"].String())
	if chatID == "" {
		return ""
	}
	if groupedID := strings.TrimSpace(event["source_grouped_id"].String()); groupedID != "" {
		return fmt.Sprintf("%s:group:%s", chatID, groupedID)
	}
	if messageID := event["source_message_id"].Int64(); messageID > 0 {
		return fmt.Sprintf("%s:message:%d", chatID, messageID)
	}
	return ""
}

func canonicalCollectSourceChatID(chatID string) string {
	return strings.TrimPrefix(strings.TrimSpace(chatID), "-100")
}

func (s *sSysPublish) filterCollectRulesByClaimedSourceGroup(ctx context.Context, event gdb.Record, rules []gdb.Record) ([]gdb.Record, error) {
	if len(rules) == 0 || collectSourceGroupKey(event) == "" {
		return rules, nil
	}
	chatID := canonicalCollectSourceChatID(event["source_chat_id"].String())
	query := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		InnerJoin(publishCollectEventTable+" e", "e.id=d.event_id").
		InnerJoin(collectDispatchChannelTable+" dc", "dc.dispatch_id=d.id").
		Fields("dc.channel_id").
		Where("d.tenant_id", event["tenant_id"].Int64()).
		Where("d.account_id", event["account_id"].Int64()).
		Where("d.status <> ?", sysin.CollectDispatchStatusFailed).
		Where("d.event_id <> ?", event["id"].Int64()).
		Where("REPLACE(e.source_chat_id, '-100', '') = ?", chatID)
	if groupedID := strings.TrimSpace(event["source_grouped_id"].String()); groupedID != "" {
		query = query.Where("e.source_grouped_id", groupedID)
	} else {
		query = query.Where("e.source_grouped_id = ''").Where("e.source_message_id", event["source_message_id"].Int64())
	}
	values, err := query.Array()
	if err != nil {
		return nil, gerror.Wrap(err, "检查来源消息组分发记录失败")
	}
	claimed := make(map[int64]struct{}, len(values))
	for _, value := range values {
		claimed[value.Int64()] = struct{}{}
	}
	filtered := make([]gdb.Record, 0, len(rules))
	for _, rule := range rules {
		available := make([]int64, 0)
		for _, channelID := range collectRuleTargetChannelIds(rule) {
			if _, exists := claimed[channelID]; !exists {
				available = append(available, channelID)
				claimed[channelID] = struct{}{}
			}
		}
		if len(available) == 0 {
			continue
		}
		rule["target_channel_ids"] = g.NewVar(available)
		filtered = append(filtered, rule)
	}
	return filtered, nil
}

func attachCollectRuleChannels(ctx context.Context, rules []gdb.Record) error {
	ruleIds := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ruleIds = append(ruleIds, rule["id"].Int64())
	}
	channelMap, err := collectRuleChannelMap(ctx, ruleIds)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		rule["target_channel_ids"] = g.NewVar(channelMap[rule["id"].Int64()])
	}
	return nil
}

func collectRuleTargetChannelIds(rule gdb.Record) []int64 {
	if rule.IsEmpty() {
		return nil
	}
	return uniqueIds(gconv.Int64s(rule["target_channel_ids"].Interface()))
}

const (
	collectRuleChannelTable     = "hg_youban_publish_collect_rule_channel"
	collectDispatchChannelTable = "hg_youban_publish_collect_dispatch_channel"
)

func collectRuleChannelMap(ctx context.Context, ruleIds []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	ruleIds = uniqueIds(ruleIds)
	if len(ruleIds) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model(collectRuleChannelTable).Safe().Ctx(ctx).
		Fields("rule_id,channel_id").WhereIn("rule_id", ruleIds).
		OrderAsc("rule_id").OrderAsc("channel_id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集规则频道失败")
	}
	for _, row := range rows {
		result[row["rule_id"].Int64()] = append(result[row["rule_id"].Int64()], row["channel_id"].Int64())
	}
	return result, nil
}

func collectDispatchChannelMap(ctx context.Context, dispatchIds []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	dispatchIds = uniqueIds(dispatchIds)
	if len(dispatchIds) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model(collectDispatchChannelTable).Safe().Ctx(ctx).
		Fields("dispatch_id,channel_id").WhereIn("dispatch_id", dispatchIds).
		OrderAsc("dispatch_id").OrderAsc("channel_id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集分发频道失败")
	}
	for _, row := range rows {
		result[row["dispatch_id"].Int64()] = append(result[row["dispatch_id"].Int64()], row["channel_id"].Int64())
	}
	return result, nil
}

func syncCollectRuleChannelsTx(ctx context.Context, tx gdb.TX, tenantId, accountId, ruleId int64, channelIds []int64) error {
	channelIds = uniqueIds(channelIds)
	if _, err := tx.Model(collectRuleChannelTable).Ctx(ctx).Where("rule_id", ruleId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集规则频道失败")
	}
	now := gtime.Now()
	for _, channelId := range channelIds {
		if _, err := tx.Model(collectRuleChannelTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "account_id": accountId, "rule_id": ruleId,
			"channel_id": channelId, "created_at": now,
		}).Insert(); err != nil {
			return gerror.Wrap(err, "保存采集规则频道失败")
		}
	}
	return nil
}

func createCollectDispatchChannelsTx(ctx context.Context, tx gdb.TX, tenantId, accountId, dispatchId int64, channelIds []int64) error {
	now := gtime.Now()
	for _, channelId := range uniqueIds(channelIds) {
		if _, err := tx.Model(collectDispatchChannelTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "account_id": accountId, "dispatch_id": dispatchId,
			"channel_id": channelId, "created_at": now,
		}).Insert(); err != nil {
			return gerror.Wrap(err, "保存采集分发频道失败")
		}
	}
	return nil
}
