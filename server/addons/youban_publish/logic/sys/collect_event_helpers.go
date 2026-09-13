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

	eventTotal, err := eventDao.Ctx(ctx).Where(eventCols.SourceId, sourceID).Count()
	if err != nil {
		return gerror.Wrap(err, "统计采集源事件数失败")
	}
	failedTotal, err := eventDao.Ctx(ctx).
		Where(eventCols.SourceId, sourceID).
		Where(eventCols.Status, sysin.CollectEventStatusFailed).
		Where("COALESCE(material_role, '') <> ?", collectMaterialRoleVerify).
		Count()
	if err != nil {
		return gerror.Wrap(err, "统计采集源失败数失败")
	}
	successTotal, err := dispatchDao.Ctx(ctx).
		Where(dispatchCols.SourceId, sourceID).
		Where(dispatchCols.Status, sysin.CollectDispatchStatusSent).
		Fields("COUNT(DISTINCT " + dispatchCols.EventId + ")").Value()
	if err != nil {
		return gerror.Wrap(err, "统计采集源成功数失败")
	}
	lastEventAt, err := eventDao.Ctx(ctx).
		Where(eventCols.SourceId, sourceID).
		Fields("MAX(" + eventCols.ReceivedAt + ")").Value()
	if err != nil {
		return gerror.Wrap(err, "统计采集源最后事件时间失败")
	}
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(sourceCols.Id, sourceID).
		Data(g.Map{
			sourceCols.EventTotal:   eventTotal,
			sourceCols.SuccessTotal: successTotal.Int64(),
			sourceCols.FailedTotal:  failedTotal,
			sourceCols.LastEventAt:  lastEventAt,
			sourceCols.UpdatedAt:    gtime.Now(),
		}).Update()
	return gerror.Wrap(err, "更新采集源统计失败")
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
	if err = s.syncCollectSourceStats(ctx, sourceID.Int64()); err != nil {
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
