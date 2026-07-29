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
	collectMaterialGroupingDelay = 3 * time.Minute
	collectMaterialVerifyWindow  = 10 * time.Minute
	collectMaterialRolePending   = "pending"
	collectMaterialRoleDisplay   = "display"
	collectMaterialRoleVerify    = "verify"
)

type collectMaterialMessageView struct {
	RawText string
	Media   []collectMediaItem
}

type collectMaterialPair struct {
	DisplayIndex int
	VerifyIndex  int
}

func pairCollectMaterialMessages(messages []collectMaterialMessageView) []collectMaterialPair {
	pairs := make([]collectMaterialPair, 0)
	for index, message := range messages {
		if classifyProfileMessage(message.RawText, message.Media).Kind != profileMessageKindDisplay {
			continue
		}
		for nextIndex := index + 1; nextIndex < len(messages); nextIndex++ {
			next := messages[nextIndex]
			switch classifyProfileMessage(next.RawText, next.Media).Kind {
			case profileMessageKindDisplay:
				nextIndex = len(messages)
			case profileMessageKindVerify:
				pairs = append(pairs, collectMaterialPair{DisplayIndex: index, VerifyIndex: nextIndex})
				nextIndex = len(messages)
			}
		}
	}
	return pairs
}

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
		}).
		WhereNull("processed_at").
		OrderAsc("source_chat_id").
		OrderAsc("source_message_id").
		OrderAsc("id").
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
		if role != "" && role != collectMaterialRolePending {
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
		switch classification.Kind {
		case profileMessageKindDisplay:
			verifyIndex := s.findCollectVerifyEvent(rows, index)
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
			if !collectMaterialEventOlderThan(event, collectMaterialVerifyWindow) {
				continue
			}
			if strings.TrimSpace(event["material_role"].String()) == collectMaterialRolePending {
				if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), "验证资料未匹配到前序资料组", "group"); err != nil {
					return err
				}
			}
		default:
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), "消息不是资料组或验证组", "group"); err != nil {
				return err
			}
		}
	}
	return nil
}

func isCollectProcessRetryError(err error) bool {
	var retryErr *collectProcessRetryError
	return errors.As(err, &retryErr)
}

func (s *sSysPublish) findCollectVerifyEvent(rows []gdb.Record, displayIndex int) int {
	views := make([]collectMaterialMessageView, 0, len(rows))
	for _, row := range rows {
		views = append(views, collectMaterialMessageView{
			RawText: row["raw_text"].String(),
			Media:   collectMediaRowsToItemsFromJSON(row["media_json"].String()),
		})
	}
	for _, pair := range pairCollectMaterialMessages(views) {
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
		Where("material_role", collectMaterialRolePending).
		Data(g.Map{
			"material_role":            collectMaterialRoleVerify,
			"material_parent_event_id": display["id"].Int64(),
			"material_group_status":    "paired",
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
