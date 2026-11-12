package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var collectGeneratedTitleNumberPattern = regexp.MustCompile(`(?im)^\s*编号\s*[:：=]\s*C\d+\s*$\n?`)

func collectPublishClientRequestId(event gdb.Record, rule gdb.Record) string {
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	chatID := normalizeTelegramChannelChatID(event["source_chat_id"].String())
	groupedID := strings.TrimSpace(event["source_grouped_id"].String())
	// Bot collectors must remain independent within the same account. Account
	// and follow-account collectors intentionally keep the shared v2 identity.
	if event["source_type"].String() == sysin.CollectSourceTypeBot && event["bot_id"].Int64() > 0 {
		botID := event["bot_id"].Int64()
		if tenantID > 0 && accountID > 0 && chatID != "" && groupedID != "" {
			return fmt.Sprintf("collect:v3:%d:%d:bot:%d:%s:group:%s", tenantID, accountID, botID, chatID, groupedID)
		}
		if messageID := event["source_message_id"].Int64(); tenantID > 0 && accountID > 0 && chatID != "" && messageID > 0 {
			return fmt.Sprintf("collect:v3:%d:%d:bot:%d:%s:message:%d", tenantID, accountID, botID, chatID, messageID)
		}
	}
	if tenantID > 0 && accountID > 0 && chatID != "" && groupedID != "" {
		return fmt.Sprintf("collect:v2:%d:%d:%s:group:%s", tenantID, accountID, chatID, groupedID)
	}
	if messageID := event["source_message_id"].Int64(); tenantID > 0 && accountID > 0 && chatID != "" && messageID > 0 {
		return fmt.Sprintf("collect:v2:%d:%d:%s:message:%d", tenantID, accountID, chatID, messageID)
	}
	return fmt.Sprintf("collect:%s:%d", event["source_unique_key"].String(), rule["id"].Int64())
}

func (s *sSysPublish) markCollectEvent(ctx context.Context, id int64, status string, message string) error {
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	data := g.Map{
		eventCols.Status:       status,
		eventCols.ErrorMessage: message,
		eventCols.UpdatedAt:    gtime.Now(),
	}
	if status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusIgnored || status == sysin.CollectEventStatusFailed {
		data[eventCols.ProcessedAt] = gtime.Now()
	}
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where(eventCols.Id, id).Data(data).Update()
	if err != nil {
		return err
	}
	if collectEventTerminalStatus(status) || status == sysin.CollectEventStatusDispatched {
		s.syncCollectSourceStatsForEvent(ctx, id)
	}
	return nil
}

// syncCollectSourceStats rebuilds counters from authoritative rows so queue
// retries and duplicate deliveries cannot inflate them.
func (s *sSysPublish) syncCollectSourceStats(ctx context.Context, sourceID int64) error {
	if sourceID <= 0 {
		return nil
	}
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	dispatchDao := pdao.YoubanPublishCollectDispatch
	dispatchCols := dispatchDao.Columns()
	sourceCols := pdao.YoubanPublishCollectSource.Columns()

	stats, err := g.DB().GetOne(ctx, fmt.Sprintf(`SELECT
		(SELECT COUNT(*) FROM %s WHERE %s=?) AS event_total,
		(SELECT COALESCE(SUM(CASE WHEN %s=? AND COALESCE(%s,'')<>? THEN 1 ELSE 0 END),0) FROM %s WHERE %s=?) AS failed_total,
		(SELECT COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END),0) FROM %s WHERE %s=?) AS blocked_total,
		(SELECT MAX(%s) FROM %s WHERE %s=?) AS last_event_at,
		(SELECT COUNT(DISTINCT %s) FROM %s WHERE %s=? AND %s=?) AS success_total`,
		eventDao.Table(), eventCols.SourceId, eventCols.Status, "material_role", eventDao.Table(), eventCols.SourceId,
		eventCols.Status, eventDao.Table(), eventCols.SourceId,
		eventCols.ReceivedAt, eventDao.Table(), eventCols.SourceId, dispatchCols.EventId, dispatchDao.Table(), dispatchCols.SourceId, dispatchCols.Status),
		sourceID, sysin.CollectEventStatusFailed, collectMaterialRoleVerify, sourceID, sysin.CollectEventStatusIgnored, sourceID, sourceID, sourceID, sysin.CollectDispatchStatusSent)
	if err != nil {
		return gerror.Wrap(err, "统计采集源事件和成功数失败")
	}
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(sourceCols.Id, sourceID).
		Data(g.Map{
			sourceCols.EventTotal:   stats["event_total"].Int64(),
			sourceCols.SuccessTotal: stats["success_total"].Int64(),
			sourceCols.FailedTotal:  stats["failed_total"].Int64(),
			"blocked_total":         stats["blocked_total"].Int64(),
			sourceCols.LastEventAt:  stats["last_event_at"],
			sourceCols.UpdatedAt:    gtime.Now(),
		}).Update()
	return gerror.Wrap(err, "更新采集源统计失败")
}

// syncCollectSourceStatsBatch aggregates all requested sources in two scans,
// instead of running one COUNT/SUM/MAX query per source.
func (s *sSysPublish) syncCollectSourceStatsBatch(ctx context.Context, sourceIDs []int64) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	type stat struct {
		total, failed, success int64
		last                   interface{}
	}
	stats := make(map[int64]*stat, len(sourceIDs))
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	rows, err := g.DB().GetAll(ctx, fmt.Sprintf(`SELECT %s AS source_id,COUNT(*) AS event_total,
 COALESCE(SUM(CASE WHEN %s=? AND COALESCE(material_role,'')<>? THEN 1 ELSE 0 END),0) AS failed_total,
 MAX(%s) AS last_event_at FROM %s WHERE %s IN (?) GROUP BY %s`, eventCols.SourceId, eventCols.Status, eventCols.ReceivedAt, eventDao.Table(), eventCols.SourceId, eventCols.SourceId), sysin.CollectEventStatusFailed, collectMaterialRoleVerify, sourceIDs)
	if err != nil {
		return gerror.Wrap(err, "批量统计采集源事件失败")
	}
	for _, row := range rows {
		stats[row["source_id"].Int64()] = &stat{total: row["event_total"].Int64(), failed: row["failed_total"].Int64(), last: row["last_event_at"]}
	}
	dispatchDao := pdao.YoubanPublishCollectDispatch
	dispatchCols := dispatchDao.Columns()
	rows, err = g.DB().GetAll(ctx, fmt.Sprintf(`SELECT %s AS source_id,COUNT(DISTINCT %s) AS success_total FROM %s WHERE %s IN (?) AND %s=? GROUP BY %s`, dispatchCols.SourceId, dispatchCols.EventId, dispatchDao.Table(), dispatchCols.SourceId, dispatchCols.Status, dispatchCols.SourceId), sourceIDs, sysin.CollectDispatchStatusSent)
	if err != nil {
		return gerror.Wrap(err, "批量统计采集源成功数失败")
	}
	for _, row := range rows {
		if v := stats[row["source_id"].Int64()]; v != nil {
			v.success = row["success_total"].Int64()
		}
	}
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	for _, id := range sourceIDs {
		v := stats[id]
		if v == nil {
			v = &stat{}
		}
		_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).Where(sourceCols.Id, id).Data(g.Map{sourceCols.EventTotal: v.total, sourceCols.SuccessTotal: v.success, sourceCols.FailedTotal: v.failed, sourceCols.LastEventAt: v.last, sourceCols.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "批量更新采集源统计失败")
		}
	}
	return nil
}

func (s *sSysPublish) syncCollectSourceStatsForEvent(ctx context.Context, eventID int64) {
	if eventID <= 0 {
		return
	}
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	sourceID, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where(eventCols.Id, eventID).
		Fields(eventCols.SourceId).
		Value()
	if err != nil {
		g.Log().Warningf(ctx, "读取采集事件统计归属失败 eventId:%d err:%+v", eventID, err)
		return
	}
	if err = s.scheduleCollectSourceStats(ctx, sourceID.Int64()); err != nil {
		g.Log().Warningf(ctx, "同步采集源统计失败 eventId:%d sourceId:%d err:%+v", eventID, sourceID.Int64(), err)
	}
}

func collectTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = collectGeneratedTitleNumberPattern.ReplaceAllString(text, "")
	if title, _, _ := materialImportTitle(text); title != "" {
		return title
	}
	runes := []rune(text)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return text
}

func collectHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
