package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	// Grouping starts immediately. The only waiting window is the twenty-minute
	// verification window; a display message without a match is published when
	// that window expires.
	collectMaterialGroupingDelay           = 0
	collectMaterialVerifyWindowDefault     = 20 * time.Minute
	collectMaterialVerifySettleDelay       = time.Minute
	collectMaterialVerifyRetryDelay        = time.Minute
	collectMaterialWaitingVerifyRetryDelay = 10 * time.Second
	collectMaterialWindowExecutionTimeout  = 90 * time.Second
	collectMaterialRolePending             = "pending"
	collectMaterialRoleDisplay             = "display"
	collectMaterialRoleVerify              = "verify"
	collectMaterialGroupCollecting         = "group_collecting"
	collectMaterialGroupWaitingVerify      = "waiting_verify"
)

// collectMaterialVerifyWindow is the single source of truth for display/verify pairing.
func collectMaterialVerifyWindow() time.Duration {
	value := strings.TrimSpace(os.Getenv("YOUBAN_COLLECT_VERIFY_WINDOW"))
	if value == "" {
		return collectMaterialVerifyWindowDefault
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return collectMaterialVerifyWindowDefault
}

func (s *sSysPublish) processCollectSourceWindow(ctx context.Context, payload collectProcessQueuePayload) error {
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return gerror.New("采集源窗口参数不完整")
	}
	batchSize := collectMaterialWindowBatchSize(ctx)
	now := gtime.Now()
	eventModel := collectProcessEventModel(ctx, payload).
		WhereIn("e.status", collectProcessWindowStatuses()).
		// Ignored events are terminal and must never re-enter the normal window.
		// Only an ignored verify event with the explicit unmatched marker remains
		// eligible for the bounded verification retry path.
		Where("((e.status <> ? AND (e.material_role IS NULL OR e.material_role = '' OR e.material_role IN (?,?))) OR (e.status = ? AND e.material_role = ? AND e.error_message = ?))", sysin.CollectEventStatusIgnored, collectMaterialRolePending, collectMaterialRoleDisplay, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(e.processed_at IS NULL AND (e.material_group_status IS NULL OR e.material_group_status <> ? OR e.updated_at <= ?)) OR (e.status = ? AND e.material_role = ? AND e.error_message = ? AND e.updated_at <= ?)", collectMaterialGroupWaitingVerify, now.Add(-collectMaterialWaitingVerifyRetryDelay), sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, now.Add(-collectMaterialVerifyRetryDelay))
	if payload.EventId > 0 {
		eventModel = eventModel.Where("e.id", payload.EventId)
	}
	rows, err := eventModel.
		Fields("e.*").
		// Oldest checked events go first. markCollectMaterialWaitingVerify updates
		// updated_at, so a large waiting set rotates fairly instead of allowing
		// the first channel/message range to occupy every batch.
		OrderAsc("e.updated_at").
		OrderAsc("e.id").
		Limit(batchSize).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集源消息窗口失败")
	}
	if len(rows) == 0 {
		return nil
	}
	for _, event := range rows {
		targetPayload := payload
		targetPayload.EventId = event["id"].Int64()
		targetPayload.SourceChatId = event["source_chat_id"].String()
		if err = s.processTargetCollectEventWindow(ctx, targetPayload, event); err != nil && !isCollectProcessRetryError(err) {
			return err
		}
	}
	return nil
}

// processTargetCollectEventWindow processes one realtime event with only the
// forward context required to pair a verification message. It deliberately
// avoids the source backlog so old events cannot delay realtime ingestion.
func (s *sSysPublish) processTargetCollectEventWindow(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record) error {
	if target.IsEmpty() || target["id"].Int64() <= 0 {
		return gerror.New("目标采集事件不存在")
	}
	messageID := target["source_message_id"].Int64()
	if messageID <= 0 {
		return gerror.Newf("目标采集事件消息ID无效 eventId:%d", target["id"].Int64())
	}
	eventAt := target["received_at"].GTime()
	if eventAt == nil || eventAt.IsZero() {
		eventAt = target["created_at"].GTime()
	}
	if eventAt == nil || eventAt.IsZero() {
		return gerror.Newf("目标采集事件时间无效 eventId:%d", target["id"].Int64())
	}

	rows, windowTruncated, err := s.loadTargetCollectMaterialWindow(ctx, payload, target, eventAt)
	if err != nil {
		return gerror.Wrap(err, "读取目标采集事件验证窗口失败")
	}
	if len(rows) == 0 || rows[0]["id"].Int64() != target["id"].Int64() {
		return newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "目标采集事件等待状态或静默期更新")
	}
	return s.processCollectMessageWindow(ctx, payload, target["source_chat_id"].String(), rows, map[int64]struct{}{target["id"].Int64(): {}}, windowTruncated)
}

func (s *sSysPublish) loadTargetCollectMaterialWindow(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record, eventAt *gtime.Time) (gdb.Result, bool, error) {
	now := gtime.Now()
	rows, err := collectProcessEventModel(ctx, payload).
		WhereIn("e.status", collectProcessWindowStatuses()).
		Where("((e.status <> ? AND (e.material_role IS NULL OR e.material_role = '' OR e.material_role IN (?,?))) OR (e.status = ? AND e.material_role = ? AND e.error_message = ?))", sysin.CollectEventStatusIgnored, collectMaterialRolePending, collectMaterialRoleDisplay, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(e.processed_at IS NULL AND (e.material_group_status IS NULL OR e.material_group_status <> ? OR e.updated_at <= ?)) OR (e.status = ? AND e.material_role = ? AND e.error_message = ? AND e.updated_at <= ?)", collectMaterialGroupWaitingVerify, now.Add(-collectMaterialWaitingVerifyRetryDelay), sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, now.Add(-collectMaterialVerifyRetryDelay)).
		Where("e.source_message_id >= ?", target["source_message_id"].Int64()).
		Where("COALESCE(e.received_at,e.created_at) <= ?", eventAt.Add(collectMaterialVerifyWindow())).
		Fields("e.*").OrderAsc("e.source_message_id").OrderAsc("e.id").All()
	if err != nil {
		return nil, false, err
	}
	return rows, false, nil
}

func (s *sSysPublish) processCollectMessageWindow(ctx context.Context, payload collectProcessQueuePayload, chatID string, rows []gdb.Record, processEventIds map[int64]struct{}, windowTruncated bool) error {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]["source_message_id"].Int64()
		right := rows[j]["source_message_id"].Int64()
		if left == right {
			return rows[i]["id"].Int64() < rows[j]["id"].Int64()
		}
		return left < right
	})
	eventIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		eventIds = append(eventIds, row["id"].Int64())
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, eventIds)
	if err != nil {
		return err
	}
	messageViews := collectMaterialEventViews(rows, mediaByEvent)
	for index := 0; index < len(rows); index++ {
		event := rows[index]
		if _, ok := processEventIds[event["id"].Int64()]; !ok {
			continue
		}
		role := strings.TrimSpace(event["material_role"].String())
		if shouldResumeClassifiedDisplayEvent(event) {
			settled, settleErr := s.collectVerifyEventsSettled(ctx, event, collectMaterialContextClosed(messageViews, index))
			if settleErr != nil {
				return settleErr
			}
			if !settled {
				return newCollectProcessRetryError(s.collectDisplayRetryDelay(ctx, event), "等待验证资料静默期结束")
			}
			if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil && !isCollectProcessRetryError(err) {
				return err
			}
			continue
		}
		if role != "" && role != collectMaterialRolePending && !collectMaterialEventNeedsPairRepair(event) {
			continue
		}
		if !collectMaterialEventOlderThan(event, collectMaterialGroupingDelay) {
			continue
		}
		classification, err := s.classifyCollectEvent(ctx, event, nil)
		if err != nil {
			return err
		}
		switch classification.Kind {
		case profileMessageKindDisplay:
			if err = s.wakePreviousCollectDisplay(ctx, payload, event); err != nil {
				return err
			}
			verifyIndexes := s.findCollectVerifyEvents(rows, messageViews, index)
			if len(verifyIndexes) > 0 {
				pairedAny := false
				for _, verifyIndex := range verifyIndexes {
					paired, pairErr := s.bindCollectMaterialPair(ctx, event, rows[verifyIndex])
					if pairErr != nil {
						return pairErr
					}
					pairedAny = pairedAny || paired
				}
				if !pairedAny {
					continue
				}
				if collectMaterialContextClosed(messageViews, index) {
					if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil && !isCollectProcessRetryError(err) {
						return err
					}
					continue
				}
				if err = s.markCollectMaterialWaitingVerify(ctx, event["id"].Int64()); err != nil {
					return err
				}
				return newCollectProcessRetryError(s.collectDisplayRetryDelay(ctx, event), "等待连续验证资料收集完成")
			}
			if collectMaterialContextClosed(messageViews, index) {
				claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleDisplay, 0, "complete")
				if claimErr != nil {
					return claimErr
				}
				if claimed {
					if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil && !isCollectProcessRetryError(err) {
						return err
					}
				}
				continue
			}
			if collectMaterialShouldWaitForVerify(event, windowTruncated) {
				if err = s.markCollectMaterialWaitingVerify(ctx, event["id"].Int64()); err != nil {
					return err
				}
				return newCollectProcessRetryError(s.collectDisplayRetryDelay(ctx, event), "等待验证资料或窗口到期")
			}
			claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleDisplay, 0, "complete")
			if claimErr != nil {
				return claimErr
			}
			if !claimed {
				continue
			}
			s.appendCollectEventLogForRecord(ctx, event, "group", "timeout_without_verify", "验证窗口到期，按无验证资料继续处理", "")
			if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
				if !isCollectProcessRetryError(err) {
					return err
				}
				g.Log().Infof(ctx, "采集资料组等待异步依赖，继续处理后续资料 eventId:%d err:%s", event["id"].Int64(), err.Error())
			}
		case profileMessageKindVerify:
			displayIndex := s.findCollectDisplayEvent(rows, messageViews, index)
			if displayIndex < 0 {
				storedDisplay, lookupErr := s.findStoredCollectDisplayEvent(ctx, event)
				if lookupErr != nil {
					return lookupErr
				}
				if !storedDisplay.IsEmpty() {
					paired, pairErr := s.bindCollectMaterialPair(ctx, storedDisplay, event)
					if pairErr != nil {
						return pairErr
					}
					if !paired {
						continue
					}
					if err = s.scheduleCollectDisplaySettlement(ctx, payload, storedDisplay, s.collectDisplayRetryDelay(ctx, storedDisplay)); err != nil {
						return err
					}
					continue
				}
			}
			if displayIndex >= 0 {
				display := rows[displayIndex]
				paired, pairErr := s.bindCollectMaterialPair(ctx, display, event)
				if pairErr != nil {
					return pairErr
				}
				if !paired {
					continue
				}
				if err = s.scheduleCollectDisplaySettlement(ctx, payload, display, s.collectDisplayRetryDelay(ctx, display)); err != nil {
					return err
				}
				continue
			}
			if !collectMaterialEventIngestOlderThan(event, collectMaterialVerifyWindow()) {
				return newCollectProcessRetryError(collectMaterialVerifyRetryDelay, "验证资料等待前序展示资料落库")
			}
			// Persist the classification before marking the event ignored. The
			// bounded retry query only admits ignored verification events; leaving
			// this as pending makes a valid video permanently unrecoverable.
			claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleVerify, 0, "unmatched")
			if claimErr != nil {
				return claimErr
			}
			if !claimed && strings.TrimSpace(event["material_role"].String()) != collectMaterialRoleVerify {
				continue
			}
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), collectMaterialVerifyUnmatchedMessage, "group"); err != nil {
				return err
			}
			g.Log().Infof(ctx, "验证资料在窗口内未找到前序展示组，已终止自动配对 eventId:%d sourceMessageId:%d window:%s", event["id"].Int64(), event["source_message_id"].Int64(), collectMaterialVerifyWindow())
			return nil
		default:
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), "消息不是资料组或验证组", "group"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) scheduleCollectDisplaySettlement(ctx context.Context, payload collectProcessQueuePayload, display gdb.Record, delay time.Duration) error {
	if display.IsEmpty() || display["id"].Int64() <= 0 {
		return nil
	}
	target := payload
	target.EventId = display["id"].Int64()
	target.SourceId = display["source_id"].Int64()
	target.SourceChatId = display["source_chat_id"].String()
	if delay < collectProcessMinimumDelay {
		delay = collectProcessMinimumDelay
	}
	_, err := s.enqueueCollectProcessDeferred(ctx, target, delay)
	return gerror.Wrap(err, "投递展示资料截止任务失败")
}

func (s *sSysPublish) wakePreviousCollectDisplay(ctx context.Context, payload collectProcessQueuePayload, current gdb.Record) error {
	previous, err := s.findStoredCollectDisplayEvent(ctx, current)
	if err != nil || previous.IsEmpty() || previous["id"].Int64() == current["id"].Int64() {
		return err
	}
	return s.scheduleCollectDisplaySettlement(ctx, payload, previous, 0)
}

func (s *sSysPublish) collectDisplayRetryDelay(ctx context.Context, display gdb.Record) time.Duration {
	deadline := collectMaterialEventAt(display).Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
	if verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64()); err == nil && len(verifies) > 0 {
		latest := collectMaterialEventAt(verifies[len(verifies)-1])
		if !latest.IsZero() {
			deadline = latest.Add(collectMaterialVerifySettleDelay)
			maximum := collectMaterialEventAt(display).Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
			if !maximum.IsZero() && maximum.Before(deadline) {
				deadline = maximum
			}
		}
	}
	delay := time.Until(deadline)
	if delay < collectProcessMinimumDelay {
		return collectProcessMinimumDelay
	}
	return delay
}

func collectMaterialShouldWaitForVerify(event gdb.Record, windowTruncated bool) bool {
	return windowTruncated || !collectMaterialEventIngestOlderThan(event, collectMaterialVerifyWindow()+collectMaterialVerifySettleDelay)
}

func shouldResumeClassifiedDisplayEvent(event gdb.Record) bool {
	if strings.TrimSpace(event["material_role"].String()) != collectMaterialRoleDisplay {
		return false
	}
	status := strings.TrimSpace(event["status"].String())
	return status != sysin.CollectEventStatusProcessed &&
		status != sysin.CollectEventStatusDispatched &&
		status != sysin.CollectEventStatusIgnored
}

const collectMaterialVerifyUnmatchedMessage = "验证资料未匹配到前序资料组"

var errCollectMaterialPairRace = errors.New("采集资料组已被并发处理")

func collectMaterialMutableStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusFailed,
	}
}

func collectMaterialEventNeedsPairRepair(event gdb.Record) bool {
	return event["status"].String() == sysin.CollectEventStatusIgnored &&
		strings.TrimSpace(event["material_role"].String()) == collectMaterialRoleVerify &&
		strings.TrimSpace(event["error_message"].String()) == collectMaterialVerifyUnmatchedMessage
}

func isCollectProcessRetryError(err error) bool {
	var retryErr *collectProcessRetryError
	return errors.As(err, &retryErr)
}

func (s *sSysPublish) findCollectVerifyEvents(rows []gdb.Record, views []collectMaterialMessageView, displayIndex int) []int {
	indexes := make([]int, 0)
	for _, pair := range pairCollectMaterialMessages(views) {
		if pair.DisplayIndex != displayIndex {
			continue
		}
		candidate := rows[pair.VerifyIndex]
		if strings.TrimSpace(candidate["material_role"].String()) != "" && strings.TrimSpace(candidate["material_role"].String()) != collectMaterialRolePending && !collectMaterialEventNeedsPairRepair(candidate) {
			continue
		}
		if !collectMaterialEventOlderThan(candidate, collectMaterialGroupingDelay) {
			continue
		}
		indexes = append(indexes, pair.VerifyIndex)
	}
	return indexes
}

func (s *sSysPublish) findStoredCollectDisplayEvent(ctx context.Context, verify gdb.Record) (gdb.Record, error) {
	messageID := verify["source_message_id"].Int64()
	if messageID <= 0 {
		return nil, nil
	}
	verifyAt := verify["received_at"].GTime()
	if verifyAt == nil {
		verifyAt = verify["created_at"].GTime()
	}
	if verifyAt == nil || verifyAt.IsZero() {
		return nil, nil
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", verify["tenant_id"].Int64()).
		Where("account_id", verify["account_id"].Int64()).
		Where("tg_account_id", verify["tg_account_id"].Int64()).
		Where("source_id", verify["source_id"].Int64()).
		Where("source_chat_id", verify["source_chat_id"].String()).
		Where("source_message_id < ?", messageID).
		Where("COALESCE(received_at,created_at) BETWEEN ? AND ?", verifyAt.Add(-collectMaterialVerifyWindow()), verifyAt).
		OrderDesc("source_message_id").OrderDesc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "回看验证资料前序展示组失败")
	}
	eventIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		eventIDs = append(eventIDs, row["id"].Int64())
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view := collectMaterialEventViews([]gdb.Record{row}, mediaByEvent)[0]
		if collectMaterialSkippableMessage(view) {
			continue
		}
		switch classifyProfileMessage(view.RawText, view.Media).Kind {
		case profileMessageKindVerify:
			continue
		case profileMessageKindDisplay:
			return row, nil
		default:
			return nil, nil
		}
	}
	return nil, nil
}

func (s *sSysPublish) findCollectDisplayEvent(rows []gdb.Record, views []collectMaterialMessageView, verifyIndex int) int {
	if verifyIndex < 0 || verifyIndex >= len(rows) {
		return -1
	}
	// The current source window is authoritative. A persisted parent may have
	// been selected by an earlier partial recovery window before a closer
	// display event became eligible for processing.
	for _, pair := range pairCollectMaterialMessages(views) {
		if pair.VerifyIndex == verifyIndex {
			return pair.DisplayIndex
		}
	}
	verify := rows[verifyIndex]
	parentID := verify["material_parent_event_id"].Int64()
	if parentID > 0 {
		for index, row := range rows {
			if row["id"].Int64() == parentID {
				return index
			}
		}
	}
	return -1
}

func truncateCollectDiagnosticText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func (s *sSysPublish) bindCollectMaterialPair(ctx context.Context, display gdb.Record, verify gdb.Record) (bool, error) {
	if display["id"].Int64() <= 0 || verify["id"].Int64() <= 0 {
		return false, nil
	}
	canonical, err := s.isCanonicalCollectMaterialPair(ctx, display, verify)
	if err != nil {
		return false, err
	}
	if !canonical {
		g.Log().Infof(ctx, "采集资料组已存在更近的展示消息，拒绝错误绑定 chat:%s displayEventId:%d verifyEventId:%d", strings.TrimSpace(display["source_chat_id"].String()), display["id"].Int64(), verify["id"].Int64())
		return false, nil
	}
	now := gtime.Now()
	paired := false
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		displayStatus := strings.TrimSpace(display["status"].String())
		displayGroupStatus := strings.TrimSpace(display["material_group_status"].String())
		displayCanReattach := (displayStatus == sysin.CollectEventStatusProcessed || displayStatus == sysin.CollectEventStatusDispatched) &&
			(displayGroupStatus == "complete" || displayGroupStatus == "paired")
		displayQuery := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where("id", display["id"].Int64()).
			WhereIn("material_role", []string{collectMaterialRolePending, collectMaterialRoleDisplay})
		if !displayCanReattach {
			displayQuery = displayQuery.WhereIn("status", collectMaterialMutableStatuses())
		}
		result, txErr := displayQuery.
			Data(g.Map{"material_role": collectMaterialRoleDisplay, "material_group_status": "paired", "updated_at": now}).Update()
		if txErr != nil {
			return gerror.Wrap(txErr, "绑定资料展示组失败")
		}
		affected, txErr := result.RowsAffected()
		if txErr != nil {
			return gerror.Wrap(txErr, "确认资料展示组绑定结果失败")
		}
		if affected == 0 {
			return nil
		}
		result, txErr = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where("id", verify["id"].Int64()).
			Where("material_role = ? OR (status = ? AND error_message = ?)", collectMaterialRolePending, sysin.CollectEventStatusIgnored, collectMaterialVerifyUnmatchedMessage).
			Data(g.Map{"status": sysin.CollectEventStatusGroupCollect, "material_role": collectMaterialRoleVerify, "material_parent_event_id": display["id"].Int64(), "material_group_status": "paired", "error_message": "", "processed_at": nil, "updated_at": now}).Update()
		if txErr != nil {
			return gerror.Wrap(txErr, "绑定资料验证组失败")
		}
		affected, txErr = result.RowsAffected()
		if txErr != nil {
			return gerror.Wrap(txErr, "确认资料验证组绑定结果失败")
		}
		if affected == 0 {
			return errCollectMaterialPairRace
		}
		if _, txErr = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
			Where("event_id", verify["id"].Int64()).Where("cache_status", collectMediaCacheCanceled).
			Data(g.Map{"cache_status": collectMediaCachePending, "error_message": "", "next_retry_at": nil, "updated_at": now}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "恢复验证媒体下载失败")
		}
		paired = true
		return nil
	})
	if errors.Is(err, errCollectMaterialPairRace) {
		g.Log().Infof(ctx, "采集验证组配对竞态已跳过 displayEventId:%d verifyEventId:%d", display["id"].Int64(), verify["id"].Int64())
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !paired {
		g.Log().Infof(ctx, "采集资料组配对竞态已跳过 displayEventId:%d verifyEventId:%d", display["id"].Int64(), verify["id"].Int64())
		return false, nil
	}
	g.Log().Infof(ctx, "采集资料组绑定完成 chat:%s displayEventId:%d verifyEventId:%d", strings.TrimSpace(display["source_chat_id"].String()), display["id"].Int64(), verify["id"].Int64())
	s.appendCollectEventLogForRecord(ctx, display, "group", "paired", "展示资料与验证资料已在同一窗口配对", fmt.Sprintf(`{"verifyEventId":%d}`, verify["id"].Int64()))
	return true, nil
}

// isCanonicalCollectMaterialPair validates against every persisted event in
// the proposed range, including terminal and temporarily ignored rows. Normal
// processing windows intentionally filter those rows, but that filter must not
// allow an older display to steal a verification video from a closer display.
func (s *sSysPublish) isCanonicalCollectMaterialPair(ctx context.Context, display, verify gdb.Record) (bool, error) {
	displayMessageID := display["source_message_id"].Int64()
	verifyMessageID := verify["source_message_id"].Int64()
	if displayMessageID <= 0 || verifyMessageID <= displayMessageID {
		return false, nil
	}
	for _, field := range []string{"tenant_id", "account_id", "tg_account_id", "source_id", "source_chat_id"} {
		if display[field].String() != verify[field].String() {
			return false, nil
		}
	}
	previous, err := s.findStoredCollectDisplayEvent(ctx, verify)
	if err != nil || previous.IsEmpty() {
		return false, err
	}
	return previous["id"].Int64() == display["id"].Int64(), nil
}

func (s *sSysPublish) markCollectMaterialRole(ctx context.Context, eventID int64, role string, parentID int64, status string) (bool, error) {
	result, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventID).
		Where("material_role", collectMaterialRolePending).
		WhereIn("status", collectMaterialMutableStatuses()).
		Data(g.Map{
			"material_role":            role,
			"material_parent_event_id": parentID,
			"material_group_status":    status,
			"updated_at":               gtime.Now(),
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "更新采集资料组角色失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, gerror.Wrap(err, "确认采集资料组角色失败")
	}
	return affected > 0, nil
}

func (s *sSysPublish) markCollectMaterialWaitingVerify(ctx context.Context, eventID int64) error {
	result, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventID).
		WhereIn("material_role", []string{collectMaterialRolePending, collectMaterialRoleDisplay}).
		Where("material_group_status IS NULL OR material_group_status = '' OR material_group_status IN (?, ?)", collectMaterialGroupWaitingVerify, collectMaterialGroupCollecting).
		Data(g.Map{
			"material_group_status": collectMaterialGroupWaitingVerify,
			"updated_at":            gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集资料验证等待状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认采集资料验证等待状态失败")
	}
	if affected > 0 {
		s.appendCollectEventLog(ctx, eventID, "group", "waiting_verify", "展示资料进入固定20分钟验证窗口", "")
	}
	return nil
}

func (s *sSysPublish) pairedCollectVerifyEvents(ctx context.Context, displayEventID int64) (gdb.Result, error) {
	if displayEventID <= 0 {
		return nil, nil
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("material_parent_event_id", displayEventID).
		Where("material_role", collectMaterialRoleVerify).
		OrderAsc("source_message_id").
		OrderAsc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料验证组列表失败")
	}
	return rows, nil
}

func (s *sSysPublish) collectVerifyEventsSettled(ctx context.Context, display gdb.Record, contextClosed bool) (bool, error) {
	if contextClosed {
		return true, nil
	}
	rows, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil || len(rows) == 0 {
		return err == nil, err
	}
	latest := collectMaterialEventAt(rows[len(rows)-1])
	if latest.IsZero() {
		return false, nil
	}
	displayAt := collectMaterialEventAt(display)
	return collectMaterialVerifyDeadlineReached(displayAt, latest, time.Now()), nil
}

func collectMaterialVerifyDeadlineReached(displayAt, latestVerifyAt, now time.Time) bool {
	if latestVerifyAt.IsZero() {
		return false
	}
	deadline := latestVerifyAt.Add(collectMaterialVerifySettleDelay)
	if !displayAt.IsZero() {
		maximum := displayAt.Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
		if maximum.Before(deadline) {
			deadline = maximum
		}
	}
	return !now.Before(deadline)
}

func (s *sSysPublish) ensureCollectPairedVerifyReady(ctx context.Context, display gdb.Record) (bool, error) {
	verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil || len(verifies) == 0 {
		return err == nil, err
	}
	ready := true
	for _, verify := range verifies {
		if !s.collectEventNeedsMediaCache(ctx, verify) {
			continue
		}
		ready = false
		if err = s.markCollectEvent(ctx, verify["id"].Int64(), sysin.CollectEventStatusMediaPending, "验证媒体缓存中"); err != nil {
			return false, err
		}
		if err = s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(verify), 0); err != nil {
			return false, err
		}
	}
	return ready, nil
}

func collectMaterialWindowBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.collect.materialWindowBatchSize", 100).Int()
	if batchSize < 20 {
		return 20
	}
	if batchSize > 200 {
		return 200
	}
	return batchSize
}

func (s *sSysPublish) mergeCollectMaterialContent(ctx context.Context, display gdb.Record, content *collectContentResult) (*collectContentResult, error) {
	if content == nil {
		return content, nil
	}
	verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil || len(verifies) == 0 {
		return content, err
	}
	verifyItems := make([]collectMediaItem, 0)
	for _, verify := range verifies {
		ready, readyErr := s.collectEventMediaReady(ctx, verify["id"].Int64())
		if readyErr != nil {
			return nil, readyErr
		}
		if !ready {
			return nil, newCollectProcessRetryError(30*time.Second, "验证资料媒体尚未就绪")
		}
		verifyRows, rowsErr := s.collectEventMediaRows(ctx, verify["id"].Int64())
		if rowsErr != nil {
			return nil, rowsErr
		}
		verifyItems = append(verifyItems, collectMediaRowsToItems(verifyRows, collectMaterialRoleVerify)...)
	}
	content.Media = mergeCollectMediaItems(
		collectMediaWithPurpose(content.Media, collectMaterialRoleDisplay),
		verifyItems,
	)
	content.MediaCount = len(content.Media)
	content.DedupeKey = collectHash(content.NormalizedText + ":" + collectMediaSignature(content.Media))
	return content, nil
}

func collectMaterialEventOlderThan(event gdb.Record, age time.Duration) bool {
	// History imports retain Telegram's original received_at. Grouping must wait
	// from local ingestion, otherwise an old album is classified before all of
	// its messages have been merged.
	eventAt := collectMaterialEventCreatedAt(event)
	if eventAt.IsZero() {
		return false
	}
	return time.Since(eventAt) >= age
}

func collectMaterialEventIngestOlderThan(event gdb.Record, age time.Duration) bool {
	eventAt := collectMaterialEventCreatedAt(event)
	if eventAt.IsZero() {
		return false
	}
	return time.Since(eventAt) >= age
}

func collectMaterialEventTime(event gdb.Record) string {
	if eventAt := collectMaterialEventAt(event); !eventAt.IsZero() {
		return eventAt.Format("2006-01-02 15:04:05 -0700")
	}
	return ""
}

func collectMaterialEventAt(event gdb.Record) time.Time {
	value := event["received_at"].GTime()
	if value == nil {
		value = event["created_at"].GTime()
	}
	if value == nil || value.IsZero() {
		return time.Time{}
	}
	// PostgreSQL `timestamp` has no timezone. The pgx driver may return it with
	// UTC attached, while the stored wall-clock value is local application time.
	// Preserve the database wall-clock fields before comparing with time.Now().
	wallClock := value.Time
	return time.Date(
		wallClock.Year(), wallClock.Month(), wallClock.Day(),
		wallClock.Hour(), wallClock.Minute(), wallClock.Second(),
		wallClock.Nanosecond(), time.Local,
	)
}

func collectMaterialEventCreatedAt(event gdb.Record) time.Time {
	value := event["created_at"].GTime()
	if value == nil {
		value = event["received_at"].GTime()
	}
	return collectMaterialDatabaseWallClock(value)
}

func collectMaterialDatabaseWallClock(value *gtime.Time) time.Time {
	if value == nil || value.IsZero() {
		return time.Time{}
	}
	wallClock := value.Time
	return time.Date(
		wallClock.Year(), wallClock.Month(), wallClock.Day(),
		wallClock.Hour(), wallClock.Minute(), wallClock.Second(),
		wallClock.Nanosecond(), time.Local,
	)
}
