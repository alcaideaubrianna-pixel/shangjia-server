package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/gtime"
	"go.opentelemetry.io/otel/attribute"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/collectobserve"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/library/tasklog"
)

const (
	collectEventRulesCacheVersionKey = "youban_publish:collect:event_rules:version"
	collectEventRulesCacheKeyPrefix  = "youban_publish:collect:event_rules:v2"
	collectEventRulesCacheTTL        = time.Minute
)

// A live event can keep refreshing its timestamp while media recovery is
// fenced on an older event page. Check only this event's overdue media.
func (s *sSysPublish) enqueueCollectEventMediaWithRecovery(ctx context.Context, event gdb.Record) error {
	payload := collectMediaQueuePayloadFromEvent(event)
	if err := s.enqueueCollectMediaCache(ctx, payload, 0); err != nil {
		return err
	}
	if event["source_type"].String() != sysin.CollectSourceTypeBot ||
		event["status"].String() != sysin.CollectEventStatusMediaPending || payload.EventId <= 0 ||
		payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 ||
		!reserveCollectMediaRecovery(ctx, payload.EventId) {
		return nil
	}
	c := pdao.YoubanPublishCollectEventMedia.Columns()
	row, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Fields(c.Id).
		Where(c.EventId, payload.EventId).Where(c.TenantId, payload.TenantId).
		Where(c.AccountId, payload.AccountId).Where(c.SourceId, payload.SourceId).
		Where(c.CacheStatus, collectMediaCachePending).
		WhereGTE(c.DownloadAttempts, collectMediaMaxRetries+1).
		WhereLTE(c.NextRetryAt, gtime.Now()).Limit(1).One()
	if err != nil {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		releaseCollectMediaRecovery(releaseCtx, payload.EventId)
		return gerror.Wrap(err, "核对到期BOT媒体重试失败")
	}
	if row.IsEmpty() {
		return nil
	}
	payload.Reason = "bot_recovery"
	queued, err := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
	if err != nil {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		releaseCollectMediaRecovery(releaseCtx, payload.EventId)
		return err
	}
	g.Log().Info(ctx, "采集事件触发到期BOT媒体恢复", g.Map{
		"event": "collect.media.due_recovery", "tenantId": payload.TenantId,
		"accountId": payload.AccountId, "sourceId": payload.SourceId,
		"eventId": payload.EventId, "mediaId": row[c.Id].Int64(), "queued": queued,
	})
	return nil
}

func (s *sSysPublish) CollectEventList(ctx context.Context, in *sysin.CollectEventListInp) (list []*sysin.CollectEventModel, totalCount int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Error(ctx, "读取采集事件列表发生panic", g.Map{
				"sourceId": func() int64 {
					if in == nil {
						return 0
					}
					return in.SourceId
				}(),
				"stage":      "collect_event_list",
				"panic":      fmt.Sprintf("%v", recovered),
				"eventCount": len(list),
			})
			list = nil
			totalCount = 0
			err = gerror.New("读取采集事件列表失败，请稍后重试")
		}
	}()
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectEventListInp{}
	}
	mod := pdao.YoubanPublishCollectEvent.DB().Model(pdao.YoubanPublishCollectEvent.Table()+" e").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=e.source_id").
		Where("e.tenant_id", account.TenantId).
		Where("e.account_id", account.Id).
		WhereGTE("e.created_at", gtime.Now().Add(-time.Duration(tasklog.RetentionDays())*24*time.Hour))
	if in.SourceId > 0 {
		mod = mod.Where("e.source_id", in.SourceId)
	}
	if in.Status != "" {
		mod = mod.Where("e.status", strings.TrimSpace(in.Status))
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("e.raw_text", "%"+keyword+"%")
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集事件失败")
	}
	g.Log().Info(ctx, "读取采集事件列表基础数据完成", g.Map{"sourceId": in.SourceId, "total": totalCount, "pageSize": in.PerPage})
	fields := "e.*,s.title AS source_title"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("e.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集事件失败")
	}
	if err = s.fillCollectEventRouting(ctx, list, account.TenantId, account.Id); err != nil {
		g.Log().Error(ctx, "读取采集事件分发信息失败", g.Map{"sourceId": in.SourceId, "eventCount": len(list), "error": err.Error()})
		return nil, 0, err
	}
	if err = s.fillCollectEventMediaCache(ctx, list); err != nil {
		g.Log().Error(ctx, "读取采集事件媒体缓存失败", g.Map{"sourceId": in.SourceId, "eventCount": len(list), "error": err.Error()})
		return nil, 0, err
	}
	if err = s.fillCollectEventProgress(ctx, list); err != nil {
		g.Log().Error(ctx, "读取采集事件处理进度失败", g.Map{"sourceId": in.SourceId, "eventCount": len(list), "error": err.Error()})
		return nil, 0, gerror.Wrap(err, "读取采集处理阶段失败")
	}
	for _, item := range list {
		item.IgnoreType = collectEventIgnoreType(item.Status, item.ErrorMessage)
	}
	return
}

func (s *sSysPublish) fillCollectEventMediaCache(ctx context.Context, list []*sysin.CollectEventModel) error {
	eventIds := make([]int64, 0, len(list))
	byId := make(map[int64]*sysin.CollectEventModel, len(list))
	for _, item := range list {
		if item != nil && item.Id > 0 {
			eventIds = append(eventIds, item.Id)
			byId[item.Id] = item
		}
	}
	if len(eventIds) == 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields("event_id,cache_status,COUNT(*) AS total").
		WhereIn("event_id", eventIds).
		Group("event_id,cache_status").All()
	if err != nil {
		return gerror.Wrap(err, "统计采集事件媒体状态失败")
	}
	summaries := make(map[int64]collectEventMediaCacheSummary, len(eventIds))
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		summary := summaries[eventId]
		count := row["total"].Int()
		summary.Total += count
		switch row["cache_status"].String() {
		case collectMediaCacheReady:
			summary.Ready += count
		case collectMediaCacheFailed:
			summary.Failed += count
		case collectMediaCacheForwarding, collectMediaCacheDownloading:
			summary.Downloading += count
		default:
			summary.Pending += count
		}
		summaries[eventId] = summary
	}
	for eventId, item := range byId {
		item.MediaCacheStatus, item.MediaCacheMessage = collectEventMediaCacheView(summaries[eventId], item.Status, item.ErrorMessage)
	}
	return nil
}

func collectEventIgnoreType(status, message string) string {
	if status != sysin.CollectEventStatusIgnored {
		return ""
	}
	if strings.Contains(message, "重复") || strings.HasPrefix(strings.TrimSpace(message), "资料库已存在相同资料") {
		return sysin.CollectEventIgnoreTypeDedupe
	}
	return sysin.CollectEventIgnoreTypeMatch
}

func (s *sSysPublish) fillCollectEventRouting(ctx context.Context, list []*sysin.CollectEventModel, tenantId, accountId int64) error {
	if len(list) == 0 {
		return nil
	}
	eventIds := make([]int64, 0, len(list))
	sourceIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		eventIds = append(eventIds, item.Id)
		sourceIds = append(sourceIds, item.SourceId)
	}
	if len(eventIds) == 0 {
		return nil
	}

	type route struct {
		ChannelIds []int64
		ReviewId   int64
		Review     string
		Dispatch   string
	}
	routes := make(map[int64]*route, len(eventIds))
	for _, id := range eventIds {
		routes[id] = &route{}
	}
	var dispatches []struct {
		Id       int64  `json:"id"`
		EventId  int64  `json:"eventId"`
		ReviewId int64  `json:"reviewId"`
		Status   string `json:"status"`
	}
	if err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		Fields("id,event_id,review_id,status").
		Scan(&dispatches); err != nil {
		return gerror.Wrap(err, "读取采集事件分发信息失败")
	}
	dispatchIds := make([]int64, 0, len(dispatches))
	for _, item := range dispatches {
		dispatchIds = append(dispatchIds, item.Id)
	}
	dispatchChannels, err := collectDispatchChannelMap(ctx, dispatchIds)
	if err != nil {
		return err
	}
	channelIds := make([]int64, 0)
	for _, item := range dispatches {
		r := routes[item.EventId]
		if r == nil {
			continue
		}
		r.ChannelIds = append(r.ChannelIds, dispatchChannels[item.Id]...)
		r.Dispatch = item.Status
		if item.ReviewId > r.ReviewId {
			r.ReviewId = item.ReviewId
		}
	}
	reviewIds := make([]int64, 0)
	for _, r := range routes {
		if r.ReviewId > 0 {
			reviewIds = append(reviewIds, r.ReviewId)
		}
	}
	if len(reviewIds) > 0 {
		var reviews []struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		}
		if err := pdao.YoubanPublishCollectReview.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", reviewIds).Fields("id,status").Scan(&reviews); err != nil {
			return gerror.Wrap(err, "读取采集事件审核信息失败")
		}
		for _, review := range reviews {
			for _, r := range routes {
				if r.ReviewId == review.Id {
					r.Review = review.Status
				}
			}
		}
	}

	// 事件尚未生成分发记录时，展示采集源绑定规则的候选目标，避免界面误认为没有配置目标。
	var bindings []struct {
		SourceId int64 `json:"sourceId"`
		RuleId   int64 `json:"ruleId"`
	}
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).
		WhereIn("source_id", sourceIds).
		Fields("source_id,rule_id").Scan(&bindings); err != nil {
		return gerror.Wrap(err, "读取采集源目标规则失败")
	}
	ruleIds := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		ruleIds = append(ruleIds, binding.RuleId)
	}
	var rules []struct {
		Id int64 `json:"id"`
	}
	if len(ruleIds) > 0 {
		if err := pdao.YoubanPublishCollectRule.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", ruleIds).WhereNull("deleted_at").
			Fields("id").Scan(&rules); err != nil {
			return gerror.Wrap(err, "读取采集规则目标失败")
		}
	}
	ruleTargetIds := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ruleTargetIds = append(ruleTargetIds, rule.Id)
	}
	ruleTargets, err := collectRuleChannelMap(ctx, ruleTargetIds)
	if err != nil {
		return err
	}
	sourceTargets := make(map[int64][]int64)
	for _, binding := range bindings {
		sourceTargets[binding.SourceId] = append(sourceTargets[binding.SourceId], ruleTargets[binding.RuleId]...)
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		r := routes[item.Id]
		if len(r.ChannelIds) == 0 {
			r.ChannelIds = sourceTargets[item.SourceId]
		}
		r.ChannelIds = uniqueIds(r.ChannelIds)
		channelIds = append(channelIds, r.ChannelIds...)
		item.TargetChannelIds = r.ChannelIds
		item.ReviewStatus = r.Review
		item.DispatchStatus = r.Dispatch
	}
	channelIds = uniqueIds(channelIds)
	if len(channelIds) == 0 {
		return nil
	}
	var channels []struct {
		Id           int64  `json:"id"`
		ChannelTitle string `json:"channelTitle"`
	}
	if err := pdao.YoubanPublishChannel.Ctx(ctx).
		Where("tenant_id", tenantId).WhereIn("id", channelIds).WhereNull("deleted_at").
		Fields("id,channel_title").Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取采集目标频道失败")
	}
	names := make(map[int64]string, len(channels))
	for _, channel := range channels {
		names[channel.Id] = strings.TrimSpace(channel.ChannelTitle)
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		for _, id := range item.TargetChannelIds {
			if name := names[id]; name != "" {
				item.TargetChannelNames = append(item.TargetChannelNames, name)
			}
		}
	}
	return nil
}

func (s *sSysPublish) CollectEventClear(ctx context.Context, in *sysin.CollectEventClearInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || in.SourceId <= 0 {
		return gerror.New("采集源ID不能为空")
	}
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("source_id", in.SourceId).
		Delete()
	return gerror.Wrap(err, "清空采集事件失败")
}

// Bind identity and local ingress observation from the current loaded event.
// Telegram dates and naive DB timestamps are never lifecycle epochs.
func collectBusinessEventContext(ctx context.Context, event, source gdb.Record, ruleID int64) context.Context {
	ctx = bindCollectEventClock(ctx, event)
	meta := collectobserve.Metadata{RuleID: ruleID}
	if !source.IsEmpty() {
		if source["account_id"].Int64() > 0 {
			meta.AccountID = source["account_id"].Int64()
		}
		known := collectobserve.From(ctx)
		if known.BotID <= 0 {
			meta.BotID = source["bot_id"].Int64()
		}
		if known.TgAccountID <= 0 {
			meta.TgAccountID = source["tg_account_id"].Int64()
		}
		if known.SourceType == "" {
			meta.SourceType = source["source_type"].String()
		}
	}
	return collectobserve.With(ctx, meta)
}

func collectBusinessStepResult(err error, fallback collectPipelineResult) collectPipelineResult {
	if err == nil {
		return fallback
	}
	var retry *collectProcessRetryError
	if errors.As(err, &retry) {
		return collectPipelineRetryable
	}
	return collectPipelineFailed
}

func (s *sSysPublish) processCollectEvent(ctx context.Context, eventId int64, tenantId int64, accountId int64) (err error) {
	eventLock := lock.NewConfig(2*time.Minute, time.Second).Mutex(fmt.Sprintf("youban_publish:collect:event:%d", eventId))
	if lockErr := eventLock.TryLock(ctx); lockErr != nil {
		if errors.Is(lockErr, lock.ErrLockFailed) {
			g.Log().Debugf(ctx, "采集事件已有实例处理，跳过本次执行 eventId:%d", eventId)
			return nil
		}
		return gerror.Wrap(lockErr, "获取采集事件处理锁失败")
	}
	defer func() { _ = eventLock.Unlock(context.Background()) }()

	eventModel := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventId).
		Where("tenant_id", tenantId)
	if accountId > 0 {
		eventModel = eventModel.Where("(account_id=? OR (account_id=0 AND source_type=?))", accountId, sysin.CollectSourceTypeBot)
	}
	event, err := eventModel.One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	if event.IsEmpty() {
		g.Log().Warningf(ctx, "采集事件不存在，跳过处理 eventId:%d tenantId:%d accountId:%d", eventId, tenantId, accountId)
		return gerror.New("采集事件不存在")
	}
	if cooldownErr := collectEventConfigurationCooldown(ctx, event); cooldownErr != nil {
		return cooldownErr
	}
	ctx = collectBusinessEventContext(ctx, event, nil, 0)
	// Old persisted events and recovery tasks bypass the delivery ingress.
	// Check the actual Telegram chat before any media, profile or dispatch work.
	ctx, blocked, guardErr := s.checkCollectEventPublishTarget(ctx, event)
	if guardErr != nil {
		return guardErr
	}
	if blocked {
		return s.ignoreCollectEvent(ctx, eventId, "采集来源是发布目标频道，禁止回流采集", "source")
	}
	authorizationParent := ctx
	ctx, finishAuthorization := startCollectPipelineStage(ctx, collectPipelineAuthorization)
	authorizationFinished := false
	authorizationResult := collectPipelineIgnored
	defer func() {
		if !authorizationFinished {
			meta := collectobserve.From(ctx)
			finishAuthorization(collectBusinessStepResult(err, authorizationResult), err,
				attribute.Int64("account.id", meta.AccountID), attribute.Int64("collect.bot_id", meta.BotID),
				attribute.Int64("collect.tg_account_id", meta.TgAccountID))
		}
	}()
	processStartedAt := time.Now()
	defer func() {
		logCollectProcessTerminal(ctx, event, processStartedAt, err)
	}()
	g.Log().Info(ctx, "采集事件处理开始", g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_pipeline_process_started",
		"stage": "process", "status": "started", "traceId": gtrace.GetTraceID(ctx),
		"collectEventId": eventId, "tenantId": tenantId, "accountId": accountId,
		"sourceId": event["source_id"].Int64(), "sourceChatId": event["source_chat_id"].String(),
		"sourceMessageId": event["source_message_id"].Int64(), "sourceGroupedId": event["source_grouped_id"].String(),
		"eventStatus": event["status"].String(), "mediaCount": event["media_count"].Int(),
	})
	g.Log().Debugf(ctx, "采集事件开始处理 eventId:%d sourceId:%d sourceMessageId:%d groupedId:%s status:%s media:%d", eventId, event["source_id"].Int64(), event["source_message_id"].Int64(), event["source_grouped_id"].String(), event["status"].String(), event["media_count"].Int())
	sourceModel := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", event["source_id"].Int64()).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at")
	if accountId > 0 {
		sourceModel = sourceModel.Where("account_id", accountId)
	}
	source, err := sourceModel.One()
	if err != nil {
		return gerror.Wrap(err, "读取采集源状态失败")
	}
	if source.IsEmpty() {
		return s.ignoreCollectEvent(ctx, eventId, "采集源不存在", "source")
	}
	ctx = collectBusinessEventContext(ctx, event, source, 0)
	// BOT 事件可能以 account_id=0 入库；解析到采集源归属后，后续规则、
	// 频道关系和资料写入仍沿用真实账户上下文。
	if accountId <= 0 {
		accountId = event["account_id"].Int64()
		if accountId <= 0 {
			accountId = source["account_id"].Int64()
		}
	}
	if source["channel_blacklisted"].Int() != 0 {
		return s.ignoreCollectEvent(ctx, eventId, "采集频道已禁用", "source")
	}
	access, accessErr := resolveCollectEventChannelAccess(ctx, event, accountId)
	if accessErr != nil {
		return accessErr
	}
	channelID := access.ChannelID
	g.Log().Info(ctx, "采集事件实际频道授权检查", g.Map{
		"event": "collect_event_channel_authorization", "tenantId": tenantId, "accountId": accountId,
		"sourceId": event["source_id"].Int64(), "collectEventId": eventId,
		"sourceChatId": event["source_chat_id"].String(), "configuredChannelId": source["channel_id"].Int64(),
		"collectChannelId": channelID, "relationId": access.Relation.Id, "allowed": access.Allowed,
		"collectEnabled": access.Relation.CollectEnabled, "relationStatus": access.Relation.Status,
		"approvalStatus": access.Relation.ApprovalStatus,
	})
	if !access.Allowed {
		g.Log().Info(ctx, "采集事件被账户频道授权拦截", g.Map{
			"event": "collect_account_channel_blocked", "tenantId": tenantId, "accountId": accountId,
			"sourceId": event["source_id"].Int64(), "collectEventId": eventId,
			"sourceChatId": event["source_chat_id"].String(), "configuredChannelId": source["channel_id"].Int64(),
			"collectChannelId": channelID, "relationId": access.Relation.Id,
			"collectEnabled": access.Relation.CollectEnabled, "relationStatus": access.Relation.Status,
			"approvalStatus": access.Relation.ApprovalStatus,
		})
		return s.ignoreCollectEvent(ctx, eventId, "账户频道采集未启用或审批已拒绝", "source")
	}
	if channelID > 0 {
		if botID := source["bot_id"].Int64(); botID > 0 {
			botRelation, relationErr := resolveCollectEventBotChannel(ctx, tenantId, botID, access)
			if relationErr != nil {
				return relationErr
			}
			g.Log().Info(ctx, "采集事件实际BOT频道关系检查", g.Map{
				"event": "collect_event_bot_channel_authorization", "collectEventId": eventId,
				"tenantId": tenantId, "accountId": accountId, "sourceId": event["source_id"].Int64(),
				"sourceChatId": event["source_chat_id"].String(), "botId": botID,
				"collectChannelId": channelID, "botChannelId": botRelation.ChannelId,
				"botRelationId": botRelation.Id, "botRelationStatus": botRelation.Status,
			})
			if botRelation.Id <= 0 {
				// Missing discovery metadata is recoverable. The account/channel
				// relation above already authorizes collection. Insert only:
				// a concurrent membership change must not be overwritten.
				_, upsertErr := pdao.YoubanPublishCollectBotChannel.Ctx(ctx).
					Data(g.Map{"tenant_id": tenantId, "bot_id": botID, "channel_id": channelID, "status": sysin.CollectBotChannelDiscovered, "bot_role": "unknown", "first_seen_at": gtime.Now(), "last_seen_at": gtime.Now(), "removed_at": nil, "removed_reason": ""}).
					InsertIgnore()
				if upsertErr != nil {
					return gerror.Wrap(upsertErr, "补建BOT频道关系失败")
				}
			} else if botRelation.Status == sysin.CollectBotChannelRemoved {
				return s.ignoreCollectEvent(ctx, eventId, "机器人已被移出采集频道", "source")
			}
		}
	}

	event["source_name_snapshot"] = source["title"]
	event["source_username_snapshot"] = source["source_username"]
	if err = s.ensureTenantVipFeature(ctx, tenantId, sysin.TenantVipFeatureCollectSource); err != nil {
		return s.ignoreCollectEvent(ctx, eventId, "VIP会员已到期，停止处理采集事件", "vip")
	}
	if source["collect_enabled"].Int() != 1 || source["status"].Int() != 1 {
		authorizationResult = collectPipelinePending
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPending, "采集源已暂停，等待恢复")
		return s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: eventId, TenantId: tenantId, AccountId: accountId, SourceId: event["source_id"].Int64(), SourceChatId: event["source_chat_id"].String(),
		}, 30*time.Second)
	}
	authorizationMeta := collectobserve.From(ctx)
	finishAuthorization(collectPipelineCompleted, nil,
		attribute.Int64("account.id", authorizationMeta.AccountID),
		attribute.Int64("collect.bot_id", authorizationMeta.BotID),
		attribute.Int64("collect.tg_account_id", authorizationMeta.TgAccountID))
	authorizationFinished = true
	ctx = collectBusinessEventContext(authorizationParent, event, source, 0)
	if collectEventAlreadyMatched(event["status"].String()) {
		needsRepair, repairErr := s.collectEventNeedsProfileRepair(ctx, eventId)
		if repairErr != nil {
			return repairErr
		}
		if !needsRepair {
			return nil
		}
		g.Log().Debugf(ctx, "采集事件存在未回写资料，重新进入分发回写 eventId:%d", eventId)
	}
	materialRole := strings.TrimSpace(event["material_role"].String())
	if materialRole == "" || materialRole == collectMaterialRolePending {
		_, finishPair := startCollectPipelineStage(ctx, collectPipelineMaterialPair)
		finishPair(collectPipelinePending, nil, attribute.String("collect.wait_reason", "pending_pair"))
		return newCollectProcessRetryError(30*time.Second, "等待资料组窗口完成")
	}
	if materialRole == collectMaterialRoleVerify {
		parentEventId := event["material_parent_event_id"].Int64()
		if parentEventId > 0 {
			parent, parentErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
				Fields("*").
				Where("id", parentEventId).
				Where("tenant_id", tenantId).
				Where("account_id", collectEventStorageAccountID(event)).
				One()
			if parentErr != nil {
				return gerror.Wrap(parentErr, "读取验证资料父展示事件失败")
			}
			if parent.IsEmpty() {
				return newCollectProcessRetryError(30*time.Second, "等待父展示资料进入处理链路")
			}
			parentStatus := strings.TrimSpace(parent["status"].String())
			mappedDuplicate := false
			if parentStatus == sysin.CollectEventStatusIgnored {
				mappedDuplicate, parentErr = collectEventCanResumeDuplicate(ctx, parent)
				if parentErr != nil {
					return parentErr
				}
			}
			if (parentStatus == sysin.CollectEventStatusIgnored && !mappedDuplicate) || parentStatus == sysin.CollectEventStatusFailed {
				message := strings.TrimSpace(parent["error_message"].String())
				if message == "" {
					message = "父展示资料已忽略"
				}
				return s.ignoreCollectEvent(ctx, eventId, message, "group")
			}
			if !mappedDuplicate && !collectDisplayEventPassedEarlyCheck(parentStatus) {
				return newCollectProcessRetryError(5*time.Second, "等待父展示资料完成规则和去重预检")
			}
		}
		mediaCtx, finishMedia := startCollectPipelineStage(ctx, collectPipelineMediaReady)
		needsMediaCache, mediaErr := s.collectEventNeedsMediaCacheResult(mediaCtx, event)
		mediaResult := collectPipelineCompleted
		if needsMediaCache {
			mediaResult = collectPipelinePending
		}
		finishMedia(collectBusinessStepResult(mediaErr, mediaResult), mediaErr)
		if mediaErr != nil {
			return gerror.Wrap(mediaErr, "检查验证资料媒体缓存状态失败")
		}
		if needsMediaCache {
			_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "验证媒体缓存中")
			return s.enqueueCollectEventMediaWithRecovery(ctx, event)
		}
		if parentEventId <= 0 {
			return newCollectProcessRetryError(30*time.Second, "验证资料缺少父展示事件，保留任务等待修复")
		}
		parent, parentErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(parentEventId).One()
		if parentErr != nil {
			return gerror.Wrap(parentErr, "读取验证资料父展示事件失败")
		}
		if parent.IsEmpty() {
			return newCollectProcessRetryError(30*time.Second, "父展示事件不存在，保留验证任务")
		}
		if parent["status"].String() == sysin.CollectEventStatusIgnored {
			mapped, readErr := collectEventCanResumeDuplicate(ctx, parent)
			if readErr != nil {
				return readErr
			}
			if mapped {
				_, readErr = pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(parentEventId).
					Where("status", sysin.CollectEventStatusIgnored).
					Data(g.Map{"status": sysin.CollectEventStatusGroupCollect, "processed_at": nil}).Update()
				if readErr != nil {
					return readErr
				}
			}
		}
		g.Log().Infof(ctx, "验证资料媒体已完成，调度父展示资料静默截止任务 verifyEventId:%d parentEventId:%d", eventId, parentEventId)
		if err = s.scheduleCollectDisplaySettlement(ctx, collectProcessQueuePayload{
			EventId: parentEventId, TenantId: tenantId, AccountId: accountId,
			SourceId: event["source_id"].Int64(), SourceChatId: event["source_chat_id"].String(),
		}, parent, 0); err != nil {
			return err
		}
		// Do not acknowledge verification before its parent's durable wakeup.
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).Data(g.Map{
			"status":                sysin.CollectEventStatusProcessed,
			"material_group_status": "ready",
			"error_message":         "",
			"processed_at":          gtime.Now(),
			"updated_at":            gtime.Now(),
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "完成验证资料组处理失败")
		}
		return nil
	}
	// 事件级预检与分支级 rule_match 分开统计，避免耗时和结果分布混杂。
	ruleCtx, finishRules := startCollectPipelineStage(ctx, collectPipelineRulePreflight)
	rules, err := s.collectEventRules(ruleCtx, event, tenantId, accountId)
	if err != nil {
		finishRules(collectBusinessStepResult(err, collectPipelineFailed), err)
		return err
	}
	preflight, err := s.preflightCollectMaterialGroup(ruleCtx, event, rules)
	if err != nil {
		finishRules(collectBusinessStepResult(err, collectPipelineFailed), err)
		return err
	}
	if len(preflight.Rules) == 0 {
		finishRules(collectPipelineIgnored, nil)
		return s.ignoreCollectEvent(ctx, eventId, preflight.Reason, preflight.Stage)
	}
	finishRules(collectPipelineCompleted, nil, attribute.Int("collect.match_count", len(preflight.Rules)))
	candidateRules := preflight.Rules
	_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPrechecked, "")
	mediaCtx, finishMedia := startCollectPipelineStage(ctx, collectPipelineMediaReady)
	verifyReady, verifyErr := s.ensureCollectPairedVerifyReady(mediaCtx, event)
	if verifyErr != nil {
		finishMedia(collectBusinessStepResult(verifyErr, collectPipelineFailed), verifyErr)
		return verifyErr
	}
	needsMediaCache, mediaErr := s.collectEventNeedsMediaCacheResult(mediaCtx, event)
	if mediaErr != nil {
		finishMedia(collectBusinessStepResult(mediaErr, collectPipelineFailed), mediaErr)
		return gerror.Wrap(mediaErr, "检查采集资料媒体缓存状态失败")
	}
	mediaResult := collectPipelineCompleted
	if needsMediaCache || !verifyReady {
		mediaResult = collectPipelinePending
	}
	if needsMediaCache {
		finishMedia(mediaResult, nil, attribute.String("collect.wait_reason", "media_cache_pending"))
	} else if !verifyReady {
		finishMedia(mediaResult, nil, attribute.String("collect.wait_reason", "verify_media_pending"))
	} else {
		finishMedia(mediaResult, nil)
	}
	if needsMediaCache {
		g.Log().Debugf(ctx, "采集事件进入媒体缓存 eventId:%d media:%d", eventId, event["media_count"].Int())
		if err = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "媒体缓存中"); err != nil {
			return err
		}
		reportCollectProgress(ctx, "media_queued", "等待媒体工作线程执行缓存任务")
		s.appendCollectEventLogForRecord(ctx, event, "media", "pending", "媒体等待缓存", "")
		return s.enqueueCollectEventMediaWithRecovery(ctx, event)
	}
	if !verifyReady {
		return newCollectProcessRetryError(30*time.Second, "等待验证资料媒体缓存完成")
	}
	reportCollectProgress(ctx, "waiting_commit", "等待资料去重提交锁")
	lockKeys := make([]string, 0, 1)
	if sourceGroupKey := collectSourceGroupKey(event); sourceGroupKey != "" {
		lockKeys = append(lockKeys, "source_group:"+sourceGroupKey)
		sort.Strings(lockKeys)
	}
	dedupeLocks := make([]*lock.Lock, 0, len(lockKeys))
	for _, signatureKey := range lockKeys {
		// 去重锁只保护资料提交的短事务，禁止在这里无限等待；
		// Redis 锁被其他事件占用时让 Outbox 延迟重入，避免整个来源队列被阻塞。
		dedupeLock := lock.NewConfig(30*time.Second, 20*time.Millisecond).WithoutWatchDog().Mutex(fmt.Sprintf(
			"youban_publish:collect:dedupe-commit:%d:%d:%d:%s",
			event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64(), collectHash(signatureKey),
		))
		if err = dedupeLock.TryLock(ctx); err != nil {
			for index := len(dedupeLocks) - 1; index >= 0; index-- {
				_ = dedupeLocks[index].Unlock(context.Background())
			}
			// 锁竞争是可恢复的调度事件，不应把事件标记为失败或长时间占住 Worker。
			return newCollectProcessRetryError(2*time.Second, "采集资料提交锁被占用，稍后重试")
		}
		dedupeLocks = append(dedupeLocks, dedupeLock)
	}
	defer func() {
		for index := len(dedupeLocks) - 1; index >= 0; index-- {
			_ = dedupeLocks[index].Unlock(context.Background())
		}
	}()
	if event["media_count"].Int() > 0 {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaReady, "")
		s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体已就绪", "")
	}
	reportCollectProgress(ctx, "extract", "解析资料正文及合并验证内容")
	contentCtx, finishContent := startCollectPipelineStage(ctx, collectPipelineContentExtract)
	content, err := s.ensureCollectContent(contentCtx, event)
	if err != nil {
		finishContent(collectBusinessStepResult(err, collectPipelineFailed), err)
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	}
	content, err = s.mergeCollectMaterialContent(contentCtx, event, content)
	if err != nil {
		finishContent(collectBusinessStepResult(err, collectPipelineFailed), err)
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	}
	finishContent(collectPipelineCompleted, nil)
	canonicalCtx, finishCanonical := startCollectPipelineStage(ctx, collectPipelineMediaReady)
	canonical, err := s.canonicalCollectProfileMedia(canonicalCtx, event, content)
	if err != nil {
		finishCanonical(collectBusinessStepResult(err, collectPipelineFailed), err)
		return err
	}
	if err = validateCollectMaterialMedia(canonical); err != nil {
		finishCanonical(collectPipelinePending, nil, attribute.String("collect.wait_reason", "media_incomplete"))
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, err.Error())
		if enqueueErr := s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(event), 0); enqueueErr != nil {
			return enqueueErr
		}
		return newCollectProcessRetryError(30*time.Second, err.Error())
	}
	finishCanonical(collectPipelineCompleted, nil)
	content = canonical
	// Preserve raw event identity for storage/settlement. Only the submission
	// branch receives the validated source's real business account.
	branchEvent, branchErr := collectEventForBranch(event, source)
	if branchErr != nil {
		return branchErr
	}
	candidateRules, err = s.filterCollectRulesByClaimedSourceGroup(ctx, branchEvent, candidateRules)
	if err != nil {
		return err
	}
	if len(candidateRules) == 0 {
		return s.ignoreCollectEvent(ctx, eventId, "同一来源消息组已生成推送任务", "source_group")
	}
	reportCollectProgress(ctx, "commit", "按匹配规则保存资料并创建分发任务")
	fanout := executeCollectRuleBranches(ctx, candidateRules, func(branchCtx context.Context, rule gdb.Record) (bool, string, error) {
		return s.dispatchCollectEventByRule(branchCtx, branchEvent, content, rule)
	})
	for _, branch := range fanout.Branches {
		if branch.Err != nil {
			g.Log().Warning(ctx, "采集规则分支未完成，其他规则继续执行", g.Map{
				"event": "collect_rule_branch_incomplete", "collectEventId": eventId,
				"tenantId": tenantId, "accountId": accountId, "sourceId": event["source_id"].Int64(),
				"ruleId": branch.RuleID, "outcome": branch.Outcome, "error": branch.Err.Error(),
			})
		}
	}
	if fanout.Err != nil {
		// Persist terminal input/validation failures before acknowledging the wakeup.
		// Mixed transient failures keep the event retryable; successful siblings
		// retain their existing dispatch/operation identities in either case.
		terminalFailure := fanout.terminalProcessingFailure()
		status := sysin.CollectEventStatusMediaReady
		var processedAt *gtime.Time
		if terminalFailure {
			status = sysin.CollectEventStatusFailed
			processedAt = gtime.Now()
		}
		cols := pdao.YoubanPublishCollectEvent.Columns()
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		failureModel := pdao.YoubanPublishCollectEvent.Ctx(saveCtx).WherePri(eventId).
			Where(cols.TenantId, tenantId).Where(cols.SourceId, event["source_id"].Int64())
		if terminalFailure {
			// An arriving album item/replay may already have reopened this row.
			// Never close that newer material using the previous attempt's error.
			failureModel = failureModel.WhereIn(cols.Status, []string{sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusFailed}).
				Where(cols.MediaCount, event[cols.MediaCount].Int()).
				Where("COALESCE("+cols.TextHash+", '')=?", event[cols.TextHash].String())
		}
		saved, saveErr := failureModel.Data(g.Map{cols.Status: status, cols.ProcessedAt: processedAt,
			cols.ErrorMessage: fanout.Err.Error()}).Update()
		cancel()
		if saveErr != nil {
			err = gerror.Wrap(saveErr, "保存采集规则未完成状态失败")
			return err
		}
		if terminalFailure {
			affected, affectedErr := saved.RowsAffected()
			if affectedErr != nil {
				return gerror.Wrap(affectedErr, "确认采集校验失败状态保存结果失败")
			}
			if affected == 0 {
				return newCollectProcessRetryError(time.Second, "采集资料已更新，重新校验最新媒体")
			}
			event[cols.Status] = g.NewVar(status)
			event[cols.ProcessedAt] = g.NewVar(processedAt)
			event[cols.ErrorMessage] = g.NewVar(fanout.Err.Error())
			s.syncCollectSourceStatsForEvent(ctx, eventId)
			return nil
		}
		if delay := fanout.configurationCooldownDelay(); delay > 0 {
			if cooldownErr := saveCollectEventConfigurationCooldown(ctx, event, fanout.Err.Error(), delay); cooldownErr != nil {
				return cooldownErr
			}
		}
		err = fanout.Err
		return err
	}
	if !fanout.Matched {
		message := "未命中规则或被屏蔽"
		if len(fanout.Reasons) > 0 {
			message = strings.Join(uniqueStrings(fanout.Reasons), "；")
		}
		return s.ignoreCollectEvent(ctx, eventId, message, "final")
	}
	g.Log().Infof(ctx, "采集事件分发完成 eventId:%d matched:%t rules:%d", eventId, fanout.Matched, len(candidateRules))
	s.appendCollectEventLogForRecord(ctx, event, "dispatch", "dispatched", "采集事件已生成分发任务", "")
	if err = s.completePairedCollectVerifyEvent(ctx, eventId); err != nil {
		return err
	}
	return s.finishCollectEventDispatchHandoff(ctx, eventId)
}

func (s *sSysPublish) completePairedCollectVerifyEvent(ctx context.Context, displayEventID int64) error {
	if displayEventID <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("material_parent_event_id", displayEventID).
		Where("material_role", collectMaterialRoleVerify).
		WhereNotIn("status", []string{sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched}).
		Data(g.Map{
			"status":                sysin.CollectEventStatusProcessed,
			"material_group_status": "ready",
			"error_message":         "",
			"processed_at":          now,
			"updated_at":            now,
		}).Update()
	return gerror.Wrap(err, "完成配对验证资料状态失败")
}

func (s *sSysPublish) ignoreCollectEvent(ctx context.Context, eventId int64, message string, stage string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "未命中规则或被屏蔽"
	}
	if stage == "" {
		stage = "precheck"
	}
	// Only rule-fanout dedupe settlement may retain a resumable identity
	// marker. Later authorization/keyword/manual rejection is authoritative.
	if stage != "final" {
		if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventId).
			Where("material_group_status", collectDuplicateGroupStatus).
			Data(g.Map{"material_group_status": "rejected"}).Update(); err != nil {
			return err
		}
	}
	s.appendCollectEventLog(ctx, eventId, stage, "ignored", message, "")
	if err := s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusIgnored, message); err != nil {
		return err
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	if _, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where(mediaCols.EventId, eventId).
		WhereIn(mediaCols.CacheStatus, []string{collectMediaCachePending, collectMediaCacheDownloading}).
		Data(g.Map{
			mediaCols.CacheStatus:  collectMediaCacheCanceled,
			mediaCols.ErrorMessage: message,
			mediaCols.UpdatedAt:    gtime.Now(),
		}).Update(); err != nil {
		return gerror.Wrap(err, "取消已忽略事件媒体失败")
	}
	return s.ignorePairedCollectVerifyEvents(ctx, eventId, message)
}

func collectDisplayEventPassedEarlyCheck(status string) bool {
	switch strings.TrimSpace(status) {
	case sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusProcessed,
		sysin.CollectEventStatusDispatched:
		return true
	default:
		return false
	}
}

func (s *sSysPublish) ignorePairedCollectVerifyEvents(ctx context.Context, displayEventId int64, message string) error {
	if displayEventId <= 0 {
		return nil
	}
	parent, readErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(displayEventId).One()
	if readErr != nil {
		return readErr
	}
	mapped, readErr := collectEventCanResumeDuplicate(ctx, parent)
	if readErr != nil {
		return readErr
	}
	if mapped {
		return nil
	}
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	verifyIds, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(eventCols.Id).
		Where("material_parent_event_id", displayEventId).
		Where("material_role", collectMaterialRoleVerify).
		WhereNotIn(eventCols.Status, []string{sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched, sysin.CollectEventStatusIgnored}).
		Array()
	if err != nil {
		return gerror.Wrap(err, "读取配对验证资料失败")
	}
	ids := make([]int64, 0, len(verifyIds))
	for _, value := range verifyIds {
		if id := value.Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		WhereIn(eventCols.Id, ids).
		Data(g.Map{
			eventCols.Status:       sysin.CollectEventStatusIgnored,
			eventCols.ErrorMessage: message,
			eventCols.ProcessedAt:  now,
			eventCols.UpdatedAt:    now,
		}).Update(); err != nil {
		return gerror.Wrap(err, "忽略配对验证资料失败")
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.EventId, ids).
		WhereIn(mediaCols.CacheStatus, []string{collectMediaCachePending, collectMediaCacheDownloading}).
		Data(g.Map{
			mediaCols.CacheStatus:  collectMediaCacheCanceled,
			mediaCols.ErrorMessage: message,
			mediaCols.UpdatedAt:    now,
		}).Update()
	for _, id := range ids {
		s.appendCollectEventLog(ctx, id, "group", "ignored", message, fmt.Sprintf("parentEventId=%d", displayEventId))
	}
	return nil
}

func collectEventAlreadyMatched(status string) bool {
	status = strings.TrimSpace(status)
	return status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusDispatched || status == "matched"
}

func (s *sSysPublish) collectEventNeedsProfileRepair(ctx context.Context, eventId int64) (bool, error) {
	row, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("id,profile_id").
		Where("event_id", eventId).
		Where("profile_id <= 0 OR (profile_id > 0 AND EXISTS (SELECT 1 FROM "+pdao.YoubanPublishCollectEvent.Table()+" verify_event WHERE verify_event.material_parent_event_id = ? AND verify_event.material_role = ?) AND EXISTS (SELECT 1 FROM hg_content_profile profile WHERE profile.id = "+pdao.YoubanPublishCollectDispatch.Table()+".profile_id AND profile.has_verification_video <> 1 AND profile.deleted_at IS NULL))", eventId, collectMaterialRoleVerify).
		One()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集资料回写状态失败")
	}
	return !row.IsEmpty(), nil
}

func (s *sSysPublish) collectEventRules(ctx context.Context, event gdb.Record, tenantId int64, accountId int64) ([]gdb.Record, error) {
	// Ingress already fans out per enabled source. The channel is shared
	// metadata, never an identity that may replace an event's owning source.
	sourceId := event["source_id"].Int64()
	if sourceId <= 0 {
		return nil, gerror.New("采集事件缺少来源，无法解析专属规则")
	}
	var err error
	cacheKey := s.collectEventRulesCacheKey(ctx, tenantId, accountId, sourceId)
	if rows, ok := collectEventRulesCacheGet(ctx, cacheKey); ok {
		// Cached rows are complete resolved snapshots. Reattaching source-local
		// items here would overwrite the account text policy merged below.
		return rows, nil
	}
	var bindRows []struct {
		RuleId int64 `json:"ruleId"`
	}
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("source_id", sourceId).
		Where("(status IS NULL OR status <> ?)", 2).
		OrderAsc("sort").
		Scan(&bindRows); err != nil {
		return nil, gerror.Wrap(err, "读取采集源绑定规则失败")
	}
	if len(bindRows) == 0 {
		return nil, gerror.Newf("采集源%d未绑定专属规则", sourceId)
	}
	ruleIds := make([]int64, 0, len(bindRows))
	for _, row := range bindRows {
		ruleIds = append(ruleIds, row.RuleId)
	}
	// 规则和绑定的历史 status 字段不再控制执行，启停由采集源统一控制。
	// Global rules are account-wide text policies. Source matching, blocking,
	// review, dedupe and target channels remain owned by the channel rule.
	globalMod := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("global_enabled", 1).
		Where("(status IS NULL OR status <> ?)", 2).
		WhereNull("deleted_at")
	globalRows, err := globalMod.OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取租户全局采集规则失败")
	}
	// Prevent source behavior from leaking out of an account-wide text policy.
	for _, row := range globalRows {
		sanitizeGlobalCollectTextPolicy(row)
	}
	rows := make(gdb.Result, 0, len(ruleIds))
	if len(ruleIds) > 0 {
		boundRows, queryErr := pdao.YoubanPublishCollectRule.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", ruleIds).
			// Rule status 2 is the explicit disabled state. Empty/legacy
			// values remain enabled under the default-on collection policy.
			Where("(status IS NULL OR status <> ?)", 2).
			WhereNull("deleted_at").
			OrderAsc("sort").OrderAsc("id").All()
		if queryErr != nil {
			return nil, gerror.Wrap(queryErr, "读取采集源绑定规则失败")
		}
		seen := make(map[int64]struct{}, len(boundRows))
		for _, row := range boundRows {
			if _, ok := seen[row["id"].Int64()]; !ok {
				seen[row["id"].Int64()] = struct{}{}
				rows = append(rows, row)
			}
		}
	}
	if len(rows) == 0 {
		return nil, gerror.Newf("采集源%d的专属规则不存在或已删除", sourceId)
	}
	for _, row := range rows {
		row[collectRuleSourceIDField] = gvar.New(sourceId)
	}
	if err = attachCollectRuleChannels(ctx, rows); err != nil {
		return nil, err
	}
	if err = attachCollectRuleItems(ctx, rows); err != nil {
		return nil, err
	}
	if err = attachCollectRuleItems(ctx, globalRows); err != nil {
		return nil, err
	}
	mergeGlobalCollectTextPolicy(rows, globalRows)
	collectEventRulesCacheSet(ctx, cacheKey, rows)
	return rows, nil
}

func (s *sSysPublish) collectEventRulesCacheKey(ctx context.Context, tenantId int64, accountId int64, sourceId int64) string {
	version := s.collectEventRulesCacheVersion(ctx, tenantId, accountId)
	return fmt.Sprintf("%s:%s:%d:%d:%d", collectEventRulesCacheKeyPrefix, version, tenantId, accountId, sourceId)
}

func collectEventRulesCacheAccountVersionKey(tenantId, accountId int64) string {
	return fmt.Sprintf("%s:%d:%d", collectEventRulesCacheVersionKey, tenantId, accountId)
}

func (s *sSysPublish) collectEventRulesCacheVersion(ctx context.Context, tenantId, accountId int64) string {
	key := collectEventRulesCacheAccountVersionKey(tenantId, accountId)
	cacheVar, err := cache.Instance().Get(ctx, key)
	if err == nil && !cacheVar.IsNil() {
		version := strings.TrimSpace(cacheVar.String())
		if version != "" {
			return version
		}
	}
	version := fmt.Sprintf("%d", gtime.Now().TimestampNano())
	_ = cache.Instance().Set(ctx, key, version, 24*time.Hour)
	return version
}

func (s *sSysPublish) refreshCollectEventRulesCache(ctx context.Context, tenantId, accountId int64) {
	_ = cache.Instance().Set(ctx, collectEventRulesCacheAccountVersionKey(tenantId, accountId), fmt.Sprintf("%d", gtime.Now().TimestampNano()), 24*time.Hour)
}

func collectEventRulesCacheGet(ctx context.Context, key string) ([]gdb.Record, bool) {
	cacheVar, err := cache.Instance().Get(ctx, key)
	if err != nil || cacheVar.IsNil() {
		return nil, false
	}
	var list []g.Map
	if err = cacheVar.Scan(&list); err != nil {
		return nil, false
	}
	return collectEventRuleMapsToRecords(list), true
}

func collectEventRulesCacheSet(ctx context.Context, key string, rows gdb.Result) {
	_ = cache.Instance().Set(ctx, key, rows.List(), collectEventRulesCacheTTL)
}

func collectEventRuleMapsToRecords(list []g.Map) []gdb.Record {
	rows := make([]gdb.Record, 0, len(list))
	for _, item := range list {
		record := gdb.Record{}
		for key, value := range item {
			record[key] = gvar.New(value)
		}
		rows = append(rows, record)
	}
	return rows
}

func (s *sSysPublish) dispatchCollectEventByRule(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record) (matched bool, reason string, branchErr error) {
	ctx = collectBusinessEventContext(ctx, event, nil, rule["id"].Int64())
	ruleParent := ctx
	// 分支级 rule_match：每条候选规则各记录一次，重试也会产生新的观测。
	// 事件级候选预检单独记录为 rule_preflight；本阶段不代表新增资料或交付数量。
	ctx, finishRule := startCollectPipelineStage(ctx, collectPipelineRuleMatch)
	ruleFinished := false
	defer func() {
		if !ruleFinished {
			finishRule(collectBusinessStepResult(branchErr, collectPipelineIgnored), branchErr)
		}
	}()
	// TargetChannelIds are database primary keys, not Telegram chat IDs.
	// Reuse this call's checked scope, or validate direct/recovery branch calls.
	ctx, blocked, guardErr := s.checkCollectEventPublishTarget(ctx, event)
	if guardErr != nil {
		return false, "", guardErr
	}
	if blocked {
		return false, "采集来源是发布目标频道，禁止回流采集", nil
	}
	if disabled, err := collectRuleChannelDisabled(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), rule["id"].Int64(), event["bot_id"].Int64(), event["source_chat_id"].String()); err != nil {
		return false, "", err
	} else if disabled {
		return false, "当前规则的来源频道已下架禁用", nil
	}
	ruleSourceID := collectRuleSourceID(event, rule)
	if suffix, ok, err := s.resolveIntroFeeChannelSuffix(ctx, event, rule); err != nil {
		return false, "", err
	} else if ok {
		rule["intro_fee_suffix"] = gvar.New(suffix)
	}
	existingDispatch, err := findCollectBranchDispatch(ctx, pdao.YoubanPublishCollectDispatch.Ctx(ctx), event, rule)
	if err != nil {
		return false, "", gerror.Wrap(err, "读取采集分发幂等记录失败")
	}
	var decision *collectRuleDecision
	if !existingDispatch.IsEmpty() {
		// This branch was already accepted. Keep its dispatch/operation and
		// render the body without reapplying today's keyword/precheck gates.
		decision = &collectRuleDecision{Matched: true, Text: renderCollectRuleText(event, content, rule), MatchJSON: existingDispatch["match_json"].String()}
	} else {
		decision = buildCollectRuleDecision(event, content, rule)
		if decision.Skipped || !decision.Matched {
			s.appendCollectEventLogForRecord(ctx, event, "rule", "skipped", decision.Reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
			return false, decision.Reason, nil
		}
	}
	classification, err := s.classifyCollectEvent(ctx, event, content)
	if err != nil {
		return false, "", err
	}
	if classification.Kind == profileMessageKindIgnore {
		const reason = "消息不包含可创建资料的正文或有效媒体"
		s.appendCollectEventLogForRecord(ctx, event, "classifier", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, reason, nil
	}
	if classification.Kind == profileMessageKindVerify {
		const reason = "验证资料组不能独立创建资料"
		s.appendCollectEventLogForRecord(ctx, event, "classifier", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, reason, nil
	}
	finishRule(collectPipelineCompleted, nil)
	ruleFinished = true
	ctx = ruleParent
	if !existingDispatch.IsEmpty() {
		return s.resumeExistingCollectDispatch(ctx, existingDispatch, event, content, rule, decision.Text)
	}
	identity, identityErr := g.DB().Model(collectIdentityTable).Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).Where("account_id", event["account_id"].Int64()).
		Where("source_id", event["source_id"].Int64()).Where("rule_id", rule["id"].Int64()).Where("event_id", event["id"].Int64()).One()
	if identityErr != nil {
		return false, "", identityErr
	}
	if !identity.IsEmpty() {
		if mergeErr := s.mergeCollectDuplicateVerification(ctx, event, rule, content); mergeErr != nil {
			return false, "", mergeErr
		}
		return false, collectDuplicateReason(&profileFingerprintDuplicateError{ProfileID: identity["profile_id"].Int64(), Layer: identity["match_layer"].String()}, false), nil
	}
	now := gtime.Now()
	workerIdentity := currentCollectWorkerIdentity(ctx)
	channelIds := collectRuleTargetChannelIds(rule)
	if len(channelIds) == 0 {
		return false, "", gerror.New("采集规则未配置目标频道")
	}
	fingerprints := buildProfileFingerprints(channelIds, decision.Text, content.Media)
	var hit *profileFingerprintDuplicateError
	var findErr error
	if collectRuleDedupeEnabled(rule) {
		if matches, similarityErr := findMediaSimilarityMatches(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), channelIds, content.Media); similarityErr != nil {
			findErr = similarityErr
		} else if match, ok := firstMediaSimilarityMatch(matches, 2); ok {
			hit = &profileFingerprintDuplicateError{ProfileID: match.ProfileID, Layer: "image_lsh"}
		}
	}
	if hit == nil && findErr == nil {
		hit, findErr = s.findCollectRuleDuplicate(ctx, event, rule, fingerprints)
	}
	if findErr != nil {
		return false, "", findErr
	}
	if hit != nil {
		if hit.Layer == "image_lsh" && duplicateCollectReplaceOn(ctx) {
			oldText, textErr := s.collectDuplicateProfileRawText(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), hit.ProfileID)
			if textErr != nil {
				return false, "", textErr
			}
			newText := strings.TrimSpace(event["raw_text"].String())
			if oldText != "" && oldText != newText {
				if replaceErr := s.replaceCollectProfileText(ctx, hit.ProfileID, event, rule, decision.Text); replaceErr != nil {
					return false, "", replaceErr
				}
				replaced, replaceErr := s.replaceCollectProfileMedia(ctx, event, content, hit.ProfileID, collectMediaReplaceDuplicate)
				if replaceErr != nil {
					return false, "", replaceErr
				}
				if !replaced {
					channels, channelErr := s.profileChannelIdsOrDefaults(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), hit.ProfileID)
					if channelErr != nil {
						return false, "", channelErr
					}
					meta := telegramProfilePublishMeta{CollectEventId: event["id"].Int64(), CollectSourceId: event["source_id"].Int64(), CollectSourceChatId: strings.TrimSpace(event["source_chat_id"].String()), CollectSourceMessageId: event["source_message_id"].Int64()}
					if publishErr := s.submitProfilePublishWithMeta(ctx, hit.ProfileID, event["tenant_id"].Int64(), event["account_id"].Int64(), event["account_id"].Int64(), fmt.Sprintf("collect_text_replace:%d", event["id"].Int64()), channels, false, meta); publishErr != nil {
						return false, "", gerror.Wrap(publishErr, "提交重复资料文案覆盖发布失败")
					}
				}
				if replaced {
					reason := collectDuplicateReason(hit, true)
					s.appendCollectEventLogForRecord(ctx, event, "dedupe", "replaced", reason, fmt.Sprintf("rule=%d rawTextChanged=true", rule["id"].Int64()))
					return true, reason, nil
				}
			}
		}
		if err = s.recordCollectDuplicateIdentity(ctx, event, rule, hit); err != nil {
			return false, "", err
		}
		if mergeErr := s.mergeCollectDuplicateVerification(ctx, event, rule, content); mergeErr != nil {
			return false, "", mergeErr
		}
		reason := collectDuplicateReason(hit, false)
		s.appendCollectEventLogForRecord(ctx, event, "dedupe", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))

		// Only this rule is skipped. The event owner settles after all rules,
		// so a duplicate in one branch cannot cancel another branch's media.
		return false, reason, nil
	}
	var dispatchId int64
	var concurrentDispatch gdb.Record
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		lockedEvent, txErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Fields("id").
			Where("id", event["id"].Int64()).
			Where("tenant_id", event["tenant_id"].Int64()).
			Where("account_id", collectEventStorageAccountID(event)).
			LockUpdate().
			One()
		if txErr != nil {
			return gerror.Wrap(txErr, "锁定采集事件分发状态失败")
		}
		if lockedEvent.IsEmpty() {
			return gerror.New("采集事件不存在，无法创建分发记录")
		}
		concurrentDispatch, txErr = findCollectBranchDispatch(ctx, tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx), event, rule)
		if txErr != nil {
			return gerror.Wrap(txErr, "复查采集分发幂等记录失败")
		}
		if !concurrentDispatch.IsEmpty() {
			return nil
		}
		dispatchId, txErr = tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).Data(g.Map{
			"tenant_id": event["tenant_id"].Int64(), "account_id": event["account_id"].Int64(),
			"source_id": ruleSourceID, "rule_id": rule["id"].Int64(), "event_id": event["id"].Int64(),
			"source_type_snapshot":       event["source_type"].String(),
			"source_name_snapshot":       event["source_name_snapshot"].String(),
			"source_username_snapshot":   event["source_username_snapshot"].String(),
			"source_chat_id_snapshot":    event["source_chat_id"].String(),
			"source_message_id_snapshot": event["source_message_id"].Int64(),
			"match_json":                 decision.MatchJSON, "status": sysin.CollectDispatchStatusPending,
			"created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if txErr != nil {
			return gerror.Wrap(txErr, "创建采集分发记录失败")
		}
		if txErr = createCollectDispatchChannelsTx(ctx, tx, event["tenant_id"].Int64(), event["account_id"].Int64(), dispatchId, channelIds); txErr != nil {
			return txErr
		}
		return nil
	})
	if err != nil {
		return false, "", err
	}
	if !concurrentDispatch.IsEmpty() {
		return s.resumeExistingCollectDispatch(ctx, concurrentDispatch, event, content, rule, decision.Text)
	}
	if _, auditErr := pdao.YoubanPublishCollectDispatch.Ctx(ctx).WherePri(dispatchId).Data(g.Map{
		"worker_role": workerIdentity.Role, "worker_instance_id": workerIdentity.InstanceID,
		"worker_revision": workerIdentity.Revision, "worker_image_digest": workerIdentity.ImageDigest,
	}).Update(); auditErr != nil {
		g.Log().Warningf(ctx, "记录采集分发Worker身份失败，不影响资料处理 dispatchId:%d err:%+v", dispatchId, auditErr)
	}
	if rule["review_enabled"].Int() == 1 {
		if err = s.createCollectReview(ctx, event, content, rule, dispatchId, decision.Text); err != nil {
			_ = s.markCollectDispatchFailed(ctx, dispatchId, err.Error())
			return false, "", err
		}
		return true, "", nil
	}
	profileId, err := s.commitObservedCollectMaterial(ctx, event, content, rule, decision.Text)
	if err != nil {
		var duplicate *profileFingerprintDuplicateError
		if errors.As(err, &duplicate) {
			reason, skipErr := s.skipCollectProfileDispatchDuplicate(ctx, dispatchId, event, content, rule, duplicate)
			if skipErr != nil {
				return false, "", skipErr
			}
			return false, reason, nil
		}
		_ = s.markCollectDispatchFailed(ctx, dispatchId, err.Error())
		return false, "", err
	}
	if err = s.submitCollectProfileDispatch(ctx, dispatchId, profileId, event); err != nil {
		_ = s.markCollectDispatchFailed(ctx, dispatchId, err.Error())
		return false, "", err
	}
	return true, "", nil
}

func (s *sSysPublish) commitObservedCollectMaterial(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (profileID int64, persistErr error) {
	ctx = collectBusinessEventContext(ctx, event, nil, rule["id"].Int64())
	ctx, finish := startCollectPipelineStage(ctx, collectPipelineProfilePersist)
	defer func() {
		result := collectBusinessStepResult(persistErr, collectPipelinePersisted)
		var duplicate *profileFingerprintDuplicateError
		if errors.As(persistErr, &duplicate) {
			result = collectPipelineDuplicate
		} else if persistErr == nil && profileID <= 0 {
			result = collectPipelineUnconfirmed
		}
		finish(result, persistErr, attribute.Int64("profile.id", profileID))
	}()
	return s.commitCollectMaterial(ctx, event, content, rule, text)
}

// findCollectRuleDuplicate is the only collection dedupe entry. Material
// grouping is already complete before this function runs, so a hit ends the
// current group and never mutates or republishes the existing profile.
func (s *sSysPublish) findCollectRuleDuplicate(ctx context.Context, event, rule gdb.Record, fingerprints []profileFingerprint) (*profileFingerprintDuplicateError, error) {
	if !collectRuleDedupeEnabled(rule) {
		return nil, nil
	}
	textFingerprints := make([]profileFingerprint, 0, 1)
	for _, fingerprint := range fingerprints {
		if fingerprint.Layer == "text_hash" {
			textFingerprints = append(textFingerprints, fingerprint)
		}
	}
	if len(textFingerprints) == 0 {
		return nil, nil
	}
	ready, err := profileFingerprintBackfillReady(ctx)
	if err != nil {
		return nil, err
	}
	if !ready && strings.EqualFold(strings.TrimSpace(os.Getenv("YOUBAN_PROFILE_FINGERPRINT_BACKFILL_ENABLED")), "1") {
		return nil, newCollectProcessRetryError(15*time.Second, "资料指纹索引正在初始化")
	}
	scoped := scopeCollectProfileFingerprints(collectPublishClientRequestId(event, rule), textFingerprints)
	hit, err := s.findProfileFingerprintDuplicate(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), event["bot_id"].Int64(), scoped, 0)
	if err != nil || hit != nil {
		return hit, err
	}
	return nil, nil
}

func collectRuleDedupeEnabled(rule gdb.Record) bool {
	value := rule["dedupe_enabled"]
	return value != nil && value.Int() == 1
}

func (s *sSysPublish) resumeExistingCollectDispatch(ctx context.Context, dispatch gdb.Record, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (bool, string, error) {
	if err := validateCollectBranchProfileOwner(ctx, dispatch, event); err != nil {
		return false, "", err
	}
	status := dispatch["status"].String()
	handedOff := false
	if status == sysin.CollectDispatchStatusPending && dispatch["profile_id"].Int64() > 0 {
		job, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Fields("id").Where("tenant_id", event["tenant_id"].Int64()).
			WhereIn("account_id", collectEventAccountScope(event)).
			Where("profile_id", dispatch["profile_id"].Int64()).
			Where("operation_no", fmt.Sprintf("collect:%d", dispatch["id"].Int64())).
			OrderAsc("account_id").Limit(1).One()
		if err != nil {
			return false, "", gerror.Wrap(err, "核对采集分支发布交接失败")
		}
		handedOff = !job.IsEmpty()
	}
	if rule["review_enabled"].Int() == 1 && dispatch["review_id"].Int64() <= 0 &&
		(status == sysin.CollectDispatchStatusPending || status == sysin.CollectDispatchStatusFailed) && !handedOff {
		return true, "", s.createCollectReview(ctx, event, content, rule, dispatch["id"].Int64(), text)
	}
	action := collectDispatchResumeActionForStatus(status)
	if status == sysin.CollectDispatchStatusPending && rule["review_enabled"].Int() != 1 {
		// A crash can leave a pending dispatch before material commit or queue
		// submission. Pending alone is not a receipt that this branch finished.
		if dispatch["profile_id"].Int64() <= 0 {
			action = collectDispatchResumeSubmit
		} else {
			if !handedOff {
				// The profile already exists; submit the same operation without
				// rebuilding media or touching completed branches.
				return true, "", s.submitCollectProfileDispatch(ctx, dispatch["id"].Int64(), dispatch["profile_id"].Int64(), event)
			}
		}
	}
	switch action {
	case collectDispatchResumeNoop:
		if status == sysin.CollectDispatchStatusSkipped {
			if err := s.mergeCollectDuplicateVerification(ctx, event, rule, content); err != nil {
				return false, "", err
			}
			return false, dispatch["error_message"].String(), nil
		}
		if dispatch["status"].String() == sysin.CollectDispatchStatusSent {
			hasLateMedia, err := collectDispatchHasLateMedia(ctx, dispatch, event)
			if err != nil {
				return false, "", err
			}
			if hasLateMedia {
				replaced, replaceErr := s.replaceLateCollectProfileMedia(ctx, event, content, dispatch["profile_id"].Int64())
				if replaceErr != nil {
					return false, "", replaceErr
				}
				if replaced {
					return true, "", nil
				}
			}
		}
		if collectContentHasVerification(content) {
			switch dispatch["status"].String() {
			case sysin.CollectDispatchStatusPending:
				profileID, err := s.upgradeCollectDispatchProfileVerification(ctx, dispatch, event, content, rule, text)
				if err != nil {
					return false, "", err
				}
				if err = s.republishSentCollectDispatchWithVerification(ctx, event, profileID); err != nil {
					return false, "", err
				}
			case sysin.CollectDispatchStatusSent:
				if err := s.upgradeSentCollectDispatchVerification(ctx, dispatch, event, content, rule, text); err != nil {
					return false, "", err
				}
			}
		}
		return true, "", nil
	case collectDispatchResumeSubmit:
		if err := s.resumeCollectProfileDispatch(ctx, dispatch, event, content, rule, text); err != nil {
			var duplicate *profileFingerprintDuplicateError
			if errors.As(err, &duplicate) {
				reason, skipErr := s.skipCollectProfileDispatchDuplicate(ctx, dispatch["id"].Int64(), event, content, rule, duplicate)
				if skipErr != nil {
					return false, "", skipErr
				}
				return false, reason, nil
			}
			return false, "", err
		}
		return true, "", nil
	default:
		return false, "", gerror.Newf("采集分发状态不支持恢复：%s", dispatch["status"].String())
	}
}

func collectDispatchHasLateMedia(ctx context.Context, dispatch, event gdb.Record) (bool, error) {
	dispatchedAt := dispatch["created_at"].GTime()
	if dispatchedAt == nil || event["id"].Int64() <= 0 {
		return false, nil
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	row, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(mediaCols.Id).
		Where(mediaCols.EventId, event["id"].Int64()).
		WhereGT(mediaCols.CreatedAt, dispatchedAt).
		Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集事件迟到媒体失败")
	}
	return !row.IsEmpty(), nil
}

func collectContentHasVerification(content *collectContentResult) bool {
	if content == nil {
		return false
	}
	for _, item := range content.Media {
		if strings.EqualFold(strings.TrimSpace(item.Purpose), collectMaterialRoleVerify) && strings.EqualFold(strings.TrimSpace(item.Type), "video") {
			return true
		}
	}
	return false
}

func (s *sSysPublish) upgradeCollectDispatchProfileVerification(ctx context.Context, dispatch, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	profileID := dispatch["profile_id"].Int64()
	if profileID <= 0 {
		return 0, gerror.New("历史验证视频修复缺少资料ID")
	}
	updatedProfileID, err := s.commitCollectMaterial(ctx, event, content, rule, text)
	if err != nil {
		return 0, gerror.Wrap(err, "补充历史资料验证视频失败")
	}
	if updatedProfileID != profileID {
		return 0, gerror.Newf("历史验证视频修复命中了其他资料，原资料:%d 新资料:%d", profileID, updatedProfileID)
	}
	return profileID, nil
}

func (s *sSysPublish) upgradeSentCollectDispatchVerification(ctx context.Context, dispatch, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) error {
	profileID, err := s.upgradeCollectDispatchProfileVerification(ctx, dispatch, event, content, rule, text)
	if err != nil {
		return err
	}
	return s.republishSentCollectDispatchWithVerification(ctx, event, profileID)
}

// republishSentCollectDispatchWithVerification replaces every previous channel
// publication with the complete profile. Historical verification repair must
// never append a standalone video because that breaks profile ordering.
func (s *sSysPublish) republishSentCollectDispatchWithVerification(ctx context.Context, event gdb.Record, profileID int64) error {
	if profileID <= 0 {
		return gerror.New("历史验证视频修复缺少资料ID")
	}
	profile, err := g.DB().Model("hg_content_profile").Ctx(ctx).
		Fields("status").Where("id", profileID).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取历史验证修复资料状态失败")
	}
	if profile.IsEmpty() {
		return gerror.Newf("历史验证修复资料不存在 profileId:%d", profileID)
	}
	if !shouldQueueLateVerification(profile["status"].Int()) {
		g.Log().Infof(ctx, "资料已下架，仅恢复验证媒体不补发频道消息 profileId:%d eventId:%d", profileID, event["id"].Int64())
		return nil
	}
	channelValues, err := pdao.YoubanPublishTgJob.Ctx(ctx).
		Fields("channel_id").
		Where("profile_id", profileID).
		Where("collect_event_id", event["id"].Int64()).
		Where("status", telegramJobStatusSent).
		Where("send_phase", telegramSendPhaseCompletedNoVerify).
		Array()
	if err != nil {
		return gerror.Wrap(err, "读取历史资料已发送频道失败")
	}
	channelIDs := make([]int64, 0, len(channelValues))
	for _, value := range channelValues {
		if channelID := value.Int64(); channelID > 0 {
			channelIDs = append(channelIDs, channelID)
		}
	}
	channelIDs = uniqueIds(channelIDs)
	if len(channelIDs) == 0 {
		return nil
	}
	operationNo := lateVerificationRepublishOperationNo(event["id"].Int64(), profileID)
	meta := telegramProfilePublishMeta{
		CollectEventId: event["id"].Int64(), CollectSourceId: event["source_id"].Int64(),
		CollectSourceChatId:    strings.TrimSpace(event["source_chat_id"].String()),
		CollectSourceMessageId: event["source_message_id"].Int64(),
	}
	jobIDs, err := s.prepareProfilePublishJobs(ctx, profileID, event["tenant_id"].Int64(), event["account_id"].Int64(), operationNo, channelIDs, false, meta)
	if err != nil {
		return gerror.Wrap(err, "创建历史验证资料完整重发任务失败")
	}
	if len(jobIDs) == 0 {
		return nil
	}
	s.enqueuePreparedProfileJobs(ctx, jobIDs, event["account_id"].Int64())
	s.appendCollectEventLogForRecord(ctx, event, "verify", "republish_queued", "历史资料已补充验证视频并创建完整重发任务", fmt.Sprintf("profileId=%d operationNo=%s", profileID, operationNo))
	return nil
}

func shouldQueueLateVerification(profileStatus int) bool {
	return profileStatus == 1
}

func lateVerificationRepublishOperationNo(eventID, profileID int64) string {
	return fmt.Sprintf("verify-misbinding-repair:%d:profile:%d", eventID, profileID)
}

func (s *sSysPublish) classifyCollectEvent(ctx context.Context, event gdb.Record, content *collectContentResult) (profileMessageClassification, error) {
	text := strings.TrimSpace(event["raw_text"].String())
	var items []collectMediaItem
	if content != nil {
		text = strings.TrimSpace(content.RawText)
		items = content.Media
	}
	if len(items) == 0 {
		rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
		if err != nil {
			return profileMessageClassification{}, err
		}
		items = collectMediaRowsToItems(rows, event["material_role"].String())
	}
	return classifyCollectEventMedia(ctx, event, text, items)
}

// Media has already been loaded by the caller, including an empty result.
// Keep this path free of database reads so window classification stays batched.
func classifyCollectEventMedia(ctx context.Context, event gdb.Record, text string, items []collectMediaItem) (profileMessageClassification, error) {
	classification := classifyProfileMessage(text, items)
	verifyCount := 0
	displayCount := 0
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Purpose)) {
		case collectMaterialRoleVerify:
			verifyCount++
		case collectMaterialRoleDisplay:
			displayCount++
		}
	}
	g.Log().Debugf(ctx, "采集事件分类完成 eventId:%d kind:%s textBytes:%d media:%d displayMedia:%d verifyMedia:%d status:%s", event["id"].Int64(), classification.Kind, len(text), len(items), displayCount, verifyCount, event["status"].String())
	return classification, nil
}

func (s *sSysPublish) collectEventMediaReady(ctx context.Context, eventId int64) (bool, error) {
	if eventId <= 0 {
		return false, nil
	}
	rows, err := s.collectEventMediaRows(ctx, eventId)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if collectMediaRowNeedsCache(row.SourceFileId, row.SourceMessageRef, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			return false, nil
		}
	}
	return len(rows) > 0, nil
}

func collectMediaJSONHasVideo(mediaJSON string) bool {
	return collectMediaJSONHasPurposeVideo(mediaJSON, "")
}

func collectMediaJSONHasPurposeVideo(mediaJSON string, purpose string) bool {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return false
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Type), "video") {
			continue
		}
		if purpose != "" && !strings.EqualFold(strings.TrimSpace(item.Purpose), purpose) {
			continue
		}
		if purpose == "" || strings.EqualFold(strings.TrimSpace(item.Purpose), purpose) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) createCollectReview(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, dispatchId int64, text string) error {
	existingReview, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("dispatch_id", dispatchId).
		OrderDesc("id").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集审核幂等记录失败")
	}
	if !existingReview.IsEmpty() {
		if existingReview["status"].String() != sysin.CollectReviewStatusPending {
			return nil
		}
		prepared, prepareErr := s.prepareCollectMaterial(ctx, event, content)
		if prepareErr != nil {
			return gerror.Wrap(prepareErr, "刷新采集审核媒体失败")
		}
		if prepared == nil || prepared.Content == nil {
			return newCollectProcessRetryError(30*time.Second, "采集审核资料尚未准备完成")
		}
		_, updateErr := pdao.YoubanPublishCollectReview.Ctx(ctx).
			Where("id", existingReview["id"].Int64()).
			Data(g.Map{
				"media_count": prepared.Content.MediaCount,
				"updated_at":  gtime.Now(),
			}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "刷新采集审核媒体数量失败")
		}
		return s.markCollectDispatchReviewing(ctx, dispatchId, existingReview["id"].Int64())
	}
	now := gtime.Now()
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return gerror.Wrap(err, "准备采集审核资料失败")
	}
	if prepared == nil || prepared.Content == nil {
		return newCollectProcessRetryError(30*time.Second, "采集审核资料尚未准备完成")
	}
	mediaCount := prepared.Content.MediaCount
	reviewId, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Data(g.Map{
		"tenant_id":   event["tenant_id"].Int64(),
		"account_id":  event["account_id"].Int64(),
		"source_id":   collectRuleSourceID(event, rule),
		"rule_id":     rule["id"].Int64(),
		"event_id":    event["id"].Int64(),
		"dispatch_id": dispatchId,
		"raw_text":    text,
		"media_count": mediaCount,
		"status":      sysin.CollectReviewStatusPending,
		"created_at":  now,
		"updated_at":  now,
	}).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "创建采集审核失败")
	}
	g.Log().Infof(ctx, "采集审核已创建 reviewId:%d eventId:%d dispatchId:%d ruleId:%d media:%d", reviewId, event["id"].Int64(), dispatchId, rule["id"].Int64(), mediaCount)
	return s.markCollectDispatchReviewing(ctx, dispatchId, reviewId)
}

func (s *sSysPublish) markCollectDispatchReviewing(ctx context.Context, dispatchId, reviewId int64) error {
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Serialize with approval/rejection and recheck after acquiring the lock.
		review, err := tx.Model(pdao.YoubanPublishCollectReview.Table()).Where("id", reviewId).Where("dispatch_id", dispatchId).LockUpdate().One()
		if err != nil {
			return err
		}
		if review.IsEmpty() || review["status"].String() != sysin.CollectReviewStatusPending {
			return nil
		}
		_, err = tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Where("id", dispatchId).
			Where("(status IN (?,?) OR (status=? AND COALESCE(review_id,0)=0))", sysin.CollectDispatchStatusFailed, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusPending).
			Where("(COALESCE(review_id,0)=0 OR review_id=?)", reviewId).Data(g.Map{
			"review_id":     reviewId,
			"status":        sysin.CollectDispatchStatusReviewing,
			"error_message": "",
			"finished_at":   nil,
			"updated_at":    gtime.Now(),
		}).Update()
		return err
	})
	return gerror.Wrap(err, "更新采集审核分发失败")
}

// logCollectProcessTerminal writes the single terminal observation for one
// collector event. It deliberately reports the handoff boundary only; Telegram
// sending remains owned by the publish worker.
func logCollectProcessTerminal(ctx context.Context, event gdb.Record, startedAt time.Time, processErr error) {
	if event.IsEmpty() || event["id"].Int64() <= 0 {
		return
	}
	eventID := event["id"].Int64()
	status := event["status"].String()
	errorMessage := event["error_message"].String()
	processed := !event["processed_at"].IsNil()
	observeCollectPipeline(ctx, collectPipelineProcess, collectPipelineProcessResult(status, processed, processErr), startedAt, processErr, collectPipelineEventAttributes(event)...)
	if processErr != nil && errorMessage == "" {
		errorMessage = processErr.Error()
	}
	retryable := false
	var retryErr *collectProcessRetryError
	if processErr != nil {
		retryable = errors.As(processErr, &retryErr)
	}
	fields := g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_pipeline_terminal",
		"stage": "process", "eventStatus": status, "traceId": gtrace.GetTraceID(ctx),
		"collectEventId": eventID, "tenantId": event["tenant_id"].Int64(), "accountId": event["account_id"].Int64(),
		"sourceId": event["source_id"].Int64(), "sourceChatId": event["source_chat_id"].String(),
		"sourceMessageId": event["source_message_id"].Int64(), "sourceGroupedId": event["source_grouped_id"].String(),
		"processed": processed, "retryable": retryable,
		"durationMs": time.Since(startedAt).Milliseconds(),
		"outcome":    "completed", "telegramExecution": "out_of_scope",
	}
	if processErr != nil {
		fields["outcome"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", processErr)
		fields["error"] = processErr.Error()
	} else if strings.TrimSpace(status) == sysin.CollectEventStatusIgnored {
		fields["outcome"] = "skipped"
		fields["skipReason"] = errorMessage
	} else if strings.TrimSpace(errorMessage) != "" {
		fields["outcome"] = "completed_with_message"
		fields["reason"] = errorMessage
	}
	if processErr != nil {
		g.Log().Warning(ctx, "采集事件处理终态", fields)
		return
	}
	g.Log().Info(ctx, "采集事件处理终态", fields)
}
