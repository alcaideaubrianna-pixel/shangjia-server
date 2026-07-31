package sys

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
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
	collectMaterialGroupingDelay    = 3 * time.Minute
	collectMaterialVerifyWindow     = 10 * time.Minute
	collectMaterialVerifyRetryDelay = time.Minute
	collectMaterialWindowBatchSize  = 500
	collectMaterialRolePending      = "pending"
	collectMaterialRoleDisplay      = "display"
	collectMaterialRoleVerify       = "verify"
)

func (s *sSysPublish) processCollectSourceWindow(ctx context.Context, payload collectProcessQueuePayload) error {
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return gerror.New("采集源窗口参数不完整")
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		WhereIn("status", []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
			sysin.CollectEventStatusIgnored,
		}).
		Where("processed_at IS NULL OR (status = ? AND material_role = ? AND error_message = ? AND updated_at <= ?)", sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, gtime.Now().Add(-collectMaterialVerifyRetryDelay)).
		OrderAsc("source_chat_id").
		OrderAsc("source_message_id").
		OrderAsc("id").
		Limit(collectMaterialWindowBatchSize).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集源消息窗口失败")
	}
	if len(rows) == 0 {
		return nil
	}
	byChat := make(map[string][]gdb.Record)
	for _, row := range rows {
		chatID := strings.TrimSpace(row["source_chat_id"].String())
		byChat[chatID] = append(byChat[chatID], row)
	}
	for chatID, chatRows := range byChat {
		if err := s.processCollectMessageWindow(ctx, payload, chatID, chatRows); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) processCollectMessageWindow(ctx context.Context, payload collectProcessQueuePayload, chatID string, rows []gdb.Record) error {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]["source_message_id"].Int64()
		right := rows[j]["source_message_id"].Int64()
		if left == right {
			return rows[i]["id"].Int64() < rows[j]["id"].Int64()
		}
		return left < right
	})
	for index := 0; index < len(rows); index++ {
		event := rows[index]
		role := strings.TrimSpace(event["material_role"].String())
		if role != "" && role != collectMaterialRolePending && !collectMaterialEventNeedsPairRepair(event) {
			continue
		}
		if !collectMaterialEventOlderThan(event, collectMaterialGroupingDelay) {
			g.Log().Debugf(ctx, "采集事件仍在分组窗口内 eventId:%d sourceId:%d messageId:%d receivedAt:%s role:%s", event["id"].Int64(), payload.SourceId, event["source_message_id"].Int64(), collectMaterialEventTime(event), role)
			continue
		}
		g.Log().Infof(ctx, "采集事件进入分组判断 eventId:%d sourceId:%d messageId:%d chat:%s role:%s", event["id"].Int64(), payload.SourceId, event["source_message_id"].Int64(), chatID, role)
		classification, err := s.classifyCollectEvent(ctx, event, nil)
		if err != nil {
			return err
		}
		g.Log().Infof(ctx, "采集消息分组分类 eventId:%d sourceId:%d chat:%s messageId:%d groupedId:%s role:%s kind:%s media:%d text:%s", event["id"].Int64(), payload.SourceId, chatID, event["source_message_id"].Int64(), event["source_grouped_id"].String(), role, classification.Kind, event["media_count"].Int(), truncateCollectDiagnosticText(event["raw_text"].String(), 80))
		switch classification.Kind {
		case profileMessageKindDisplay:
			verifyIndex := s.findCollectVerifyEvent(rows, index)
			if verifyIndex >= 0 {
				verify := rows[verifyIndex]
				g.Log().Infof(ctx, "采集消息匹配验证组 eventId:%d verifyEventId:%d displayMessageId:%d verifyMessageId:%d verifyGroupedId:%s verifyMedia:%d", event["id"].Int64(), verify["id"].Int64(), event["source_message_id"].Int64(), verify["source_message_id"].Int64(), verify["source_grouped_id"].String(), verify["media_count"].Int())
			} else {
				g.Log().Warningf(ctx, "采集消息未匹配验证组 eventId:%d sourceMessageId:%d media:%d age:%s", event["id"].Int64(), event["source_message_id"].Int64(), event["media_count"].Int(), collectMaterialEventTime(event))
			}
			if verifyIndex >= 0 {
				if err = s.bindCollectMaterialPair(ctx, event, rows[verifyIndex]); err != nil {
					return err
				}
				if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
					if !isCollectProcessRetryError(err) {
						return err
					}
					g.Log().Infof(ctx, "采集资料组等待异步依赖，继续处理后续资料 eventId:%d err:%s", event["id"].Int64(), err.Error())
				}
				continue
			}
			if !collectMaterialEventOlderThan(event, collectMaterialVerifyWindow) {
				continue
			}
			if err = s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleDisplay, 0, "complete"); err != nil {
				return err
			}
			if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
				if !isCollectProcessRetryError(err) {
					return err
				}
				g.Log().Infof(ctx, "采集资料组等待异步依赖，继续处理后续资料 eventId:%d err:%s", event["id"].Int64(), err.Error())
			}
		case profileMessageKindVerify:
			g.Log().Infof(ctx, "采集消息识别为验证组 eventId:%d sourceMessageId:%d groupedId:%s media:%d role:%s", event["id"].Int64(), event["source_message_id"].Int64(), event["source_grouped_id"].String(), event["media_count"].Int(), role)
			displayIndex := s.findCollectDisplayEvent(rows, index)
			if displayIndex >= 0 {
				display := rows[displayIndex]
				if err = s.bindCollectMaterialPair(ctx, display, event); err != nil {
					return err
				}
				if err = s.processCollectEvent(ctx, display["id"].Int64(), payload.TenantId, payload.AccountId); err != nil && !isCollectProcessRetryError(err) {
					return err
				}
				continue
			}
			if !collectMaterialEventOlderThan(event, collectMaterialVerifyWindow) {
				continue
			}
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), collectMaterialVerifyUnmatchedMessage, "group"); err != nil {
				return err
			}
			g.Log().Infof(ctx, "验证资料暂未找到前序展示组，延迟后重试 eventId:%d sourceMessageId:%d retryAfter:%s", event["id"].Int64(), event["source_message_id"].Int64(), collectMaterialVerifyRetryDelay)
		default:
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), "消息不是资料组或验证组", "group"); err != nil {
				return err
			}
		}
	}
	return nil
}

const collectMaterialVerifyUnmatchedMessage = "验证资料未匹配到前序资料组"

func collectMaterialEventNeedsPairRepair(event gdb.Record) bool {
	return event["status"].String() == sysin.CollectEventStatusIgnored &&
		strings.TrimSpace(event["error_message"].String()) == collectMaterialVerifyUnmatchedMessage
}

func isCollectProcessRetryError(err error) bool {
	var retryErr *collectProcessRetryError
	return errors.As(err, &retryErr)
}

func (s *sSysPublish) findCollectVerifyEvent(rows []gdb.Record, displayIndex int) int {
	for _, pair := range pairCollectMaterialMessages(collectMaterialEventViews(rows)) {
		if pair.DisplayIndex != displayIndex {
			continue
		}
		candidate := rows[pair.VerifyIndex]
		if strings.TrimSpace(candidate["material_role"].String()) != "" && strings.TrimSpace(candidate["material_role"].String()) != collectMaterialRolePending {
			return -1
		}
		if !collectMaterialEventOlderThan(candidate, collectMaterialGroupingDelay) {
			return -1
		}
		return pair.VerifyIndex
	}
	return -1
}

func (s *sSysPublish) findCollectDisplayEvent(rows []gdb.Record, verifyIndex int) int {
	if verifyIndex < 0 || verifyIndex >= len(rows) {
		return -1
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
	for _, pair := range pairCollectMaterialMessages(collectMaterialEventViews(rows)) {
		if pair.VerifyIndex == verifyIndex {
			return pair.DisplayIndex
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

func (s *sSysPublish) bindCollectMaterialPair(ctx context.Context, display gdb.Record, verify gdb.Record) error {
	if display["id"].Int64() <= 0 || verify["id"].Int64() <= 0 {
		return nil
	}
	now := gtime.Now()
	if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", display["id"].Int64()).
		Where("material_role", collectMaterialRolePending).
		Data(g.Map{
			"material_role":         collectMaterialRoleDisplay,
			"material_group_status": "paired",
			"updated_at":            now,
		}).Update(); err != nil {
		return gerror.Wrap(err, "绑定资料展示组失败")
	}
	if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", verify["id"].Int64()).
		Where("material_role = ? OR (status = ? AND error_message = ?)", collectMaterialRolePending, sysin.CollectEventStatusIgnored, collectMaterialVerifyUnmatchedMessage).
		Data(g.Map{
			"status":                   sysin.CollectEventStatusGroupCollect,
			"material_role":            collectMaterialRoleVerify,
			"material_parent_event_id": display["id"].Int64(),
			"material_group_status":    "paired",
			"error_message":            "",
			"processed_at":             nil,
			"updated_at":               now,
		}).Update(); err != nil {
		return gerror.Wrap(err, "绑定资料验证组失败")
	}
	g.Log().Infof(ctx, "采集资料组绑定完成 chat:%s displayEventId:%d verifyEventId:%d", strings.TrimSpace(display["source_chat_id"].String()), display["id"].Int64(), verify["id"].Int64())
	return nil
}

func (s *sSysPublish) markCollectMaterialRole(ctx context.Context, eventID int64, role string, parentID int64, status string) error {
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventID).Data(g.Map{
		"material_role":            role,
		"material_parent_event_id": parentID,
		"material_group_status":    status,
		"updated_at":               gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "更新采集资料组角色失败")
}

func (s *sSysPublish) pairedCollectVerifyEvent(ctx context.Context, displayEventID int64) (gdb.Record, error) {
	if displayEventID <= 0 {
		return nil, nil
	}
	row, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("material_parent_event_id", displayEventID).
		Where("material_role", collectMaterialRoleVerify).
		OrderAsc("source_message_id").
		OrderAsc("id").
		Limit(1).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料验证组失败")
	}
	return row, nil
}

func (s *sSysPublish) ensureCollectPairedVerifyReady(ctx context.Context, display gdb.Record) (bool, error) {
	verify, err := s.pairedCollectVerifyEvent(ctx, display["id"].Int64())
	if err != nil || verify.IsEmpty() {
		return err == nil, err
	}
	if !s.collectEventNeedsMediaCache(ctx, verify) {
		return true, nil
	}
	if err = s.markCollectEvent(ctx, verify["id"].Int64(), sysin.CollectEventStatusMediaPending, "验证媒体缓存中"); err != nil {
		return false, err
	}
	if err = s.enqueueCollectMediaCache(ctx, collectMediaQueuePayload{
		EventId:     verify["id"].Int64(),
		TenantId:    verify["tenant_id"].Int64(),
		AccountId:   verify["account_id"].Int64(),
		SourceId:    verify["source_id"].Int64(),
		TgAccountId: verify["tg_account_id"].Int64(),
	}, 0); err != nil {
		return false, err
	}
	return false, nil
}

func (s *sSysPublish) mergeCollectMaterialContent(ctx context.Context, display gdb.Record, content *collectContentResult) (*collectContentResult, error) {
	if content == nil {
		return content, nil
	}
	verify, err := s.pairedCollectVerifyEvent(ctx, display["id"].Int64())
	if err != nil || verify.IsEmpty() {
		return content, err
	}
	ready, err := s.collectEventMediaReady(ctx, verify["id"].Int64())
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, newCollectProcessRetryError(30*time.Second, "验证资料媒体尚未就绪")
	}
	displayJSON := collectMediaJSONWithPurpose(content.MediaJSON, collectMaterialRoleDisplay)
	verifyJSON := collectMediaJSONWithPurpose(verify["media_json"].String(), collectMaterialRoleVerify)
	mediaJSON, mediaCount := mergeCollectMediaJSON(displayJSON, verifyJSON)
	content.MediaJSON = mediaJSON
	content.MediaCount = mediaCount
	content.DedupeKey = collectHash(content.NormalizedText + ":" + collectMediaSignature(mediaJSON))
	return content, nil
}

func collectMaterialEventOlderThan(event gdb.Record, age time.Duration) bool {
	eventAt := collectMaterialEventAt(event)
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

func collectMediaRowsToItemsFromJSON(mediaJSON string) []collectMediaItem {
	items := make([]collectMediaItem, 0)
	_ = json.Unmarshal([]byte(mediaJSON), &items)
	return items
}
