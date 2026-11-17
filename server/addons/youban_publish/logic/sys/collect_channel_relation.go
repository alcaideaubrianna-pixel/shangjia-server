package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	pdao "hotgo/addons/youban_publish/internal/dao"
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
	tenantID, accountID, sourceID := event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64()
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 {
		return nil, gerror.New("来源消息组分发归属不完整")
	}
	// The branch event carries the business owner. Historical Bot events and
	// their dispatch claims may still use transport account 0; the helper
	// preserves that raw scope without substituting it for the business owner.
	accountScope := collectEventAccountScope(event)
	const candidateLimit = 128
	const claimLimit = 4096
	if len(rules) > candidateLimit {
		return nil, gerror.New("来源消息组规则超过安全查询上限")
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	query := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(e.Id).Where(e.TenantId, tenantID).WhereIn(e.AccountId, accountScope).
		Where(e.SourceId, sourceID).Where(e.Id+" <> ?", event["id"].Int64()).
		WhereIn(e.SourceChatId, tgChannelCacheLookupIds(event["source_chat_id"].String()))
	if groupedID := strings.TrimSpace(event["source_grouped_id"].String()); groupedID != "" {
		query = query.Where(e.SourceGroupedId, groupedID)
	} else {
		query = query.Where(e.SourceGroupedId, "").Where(e.SourceMessageId, event["source_message_id"].Int64())
	}
	values, err := query.OrderAsc(e.Id).Limit(candidateLimit + 1).Array()
	if err != nil {
		return nil, gerror.Wrap(err, "检查同来源消息组事件失败")
	}
	if len(values) > candidateLimit {
		return nil, gerror.New("同来源消息组事件超过安全查询上限")
	}
	if len(values) == 0 {
		return rules, nil
	}
	eventIDs := make([]int64, 0, len(values))
	for _, value := range values {
		eventIDs = append(eventIDs, value.Int64())
	}
	ruleIDs := make([]int64, 0, len(rules))
	for _, rule := range rules {
		if rule["id"].Int64() <= 0 || collectRuleSourceID(event, rule) != sourceID {
			return nil, gerror.New("采集规则与事件来源归属不一致")
		}
		ruleIDs = append(ruleIDs, rule["id"].Int64())
	}
	rows, err := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		InnerJoin(collectDispatchChannelTable+" dc", "dc.dispatch_id=d.id").
		Fields("d.rule_id,dc.channel_id").
		Where("d.tenant_id", tenantID).WhereIn("d.account_id", accountScope).Where("d.source_id", sourceID).
		WhereIn("d.event_id", eventIDs).WhereIn("d.rule_id", uniqueIds(ruleIDs)).
		Where("d.status <> ?", sysin.CollectDispatchStatusFailed).
		OrderAsc("d.id").OrderAsc("dc.channel_id").Limit(claimLimit + 1).All()
	if err != nil {
		return nil, gerror.Wrap(err, "检查来源消息组分发记录失败")
	}
	if len(rows) > claimLimit {
		return nil, gerror.New("来源消息组分发记录超过安全查询上限")
	}
	return filterCollectRulesByBranchClaims(rules, rows), nil
}

// Claims are already scoped to tenant/account/source. Rule A never reserves a
// channel for rule B, including when neither has a persisted dispatch yet.
func filterCollectRulesByBranchClaims(rules []gdb.Record, claims []gdb.Record) []gdb.Record {
	claimed := make(map[int64]map[int64]bool)
	for _, claim := range claims {
		ruleID := claim["rule_id"].Int64()
		if claimed[ruleID] == nil {
			claimed[ruleID] = make(map[int64]bool)
		}
		claimed[ruleID][claim["channel_id"].Int64()] = true
	}
	filtered := make([]gdb.Record, 0, len(rules))
	for _, rule := range rules {
		available := make([]int64, 0)
		for _, channelID := range collectRuleTargetChannelIds(rule) {
			if !claimed[rule["id"].Int64()][channelID] {
				available = append(available, channelID)
			}
		}
		if len(available) == 0 {
			continue
		}
		copyRule := make(gdb.Record, len(rule))
		for key, value := range rule {
			copyRule[key] = value
		}
		copyRule["target_channel_ids"] = g.NewVar(available)
		filtered = append(filtered, copyRule)
	}
	return filtered
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
	if len(channelIds) > 0 {
		validCount, err := tx.Model(pdao.YoubanPublishChannel.Table()).Ctx(ctx).
			Where("tenant_id", tenantId).
			WhereIn("id", channelIds).
			Where("status", 1).
			WhereNull("deleted_at").Count()
		if err != nil {
			g.Log().Warningf(ctx, "采集规则目标频道校验查询失败 tenantId:%d accountId:%d ruleId:%d channelIds:%v err:%+v", tenantId, accountId, ruleId, channelIds, err)
			return gerror.Wrap(err, "校验采集规则目标频道失败")
		}
		if validCount != len(channelIds) {
			g.Log().Warningf(ctx, "采集规则目标频道不合法 tenantId:%d accountId:%d ruleId:%d channelIds:%v validCount:%d", tenantId, accountId, ruleId, channelIds, validCount)
			return gerror.New("采集规则包含不存在、停用或不属于当前租户的目标频道")
		}
	}
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
