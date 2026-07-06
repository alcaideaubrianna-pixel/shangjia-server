package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) collectBotMessage(ctx context.Context, botId int64, msg *models.Message) {
	if msg == nil {
		return
	}
	sources, err := s.collectSourcesByBotMessage(ctx, botId, msg)
	if err != nil {
		g.Log().Warningf(ctx, "读取Bot采集源失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
		return
	}
	for _, source := range sources {
		eventId, err := s.saveCollectBotEvent(ctx, source, botId, msg)
		if err != nil {
			g.Log().Warningf(ctx, "保存Bot采集事件失败 source:%d msg:%d err:%+v", gconv.Int64(source["id"]), msg.ID, err)
			continue
		}
		if err = s.processCollectEvent(ctx, eventId, gconv.Int64(source["tenant_id"]), gconv.Int64(source["account_id"])); err != nil {
			g.Log().Warningf(ctx, "处理Bot采集事件失败 event:%d err:%+v", eventId, err)
		}
	}
}

func (s *sSysPublish) collectSourcesByBotMessage(ctx context.Context, botId int64, msg *models.Message) ([]g.Map, error) {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	mod := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("source_type", sysin.CollectSourceTypeBot).
		Where("collect_enabled", 1).
		Where("status", 1).
		Where("source_chat_id", chatId).
		WhereNull("deleted_at")
	if botId > 0 {
		mod = mod.Where("bot_id", botId)
	}
	records, err := mod.All()
	if err != nil {
		return nil, err
	}
	rows := make([]g.Map, 0, len(records))
	for _, record := range records {
		rows = append(rows, record.Map())
	}
	return rows, nil
}

func (s *sSysPublish) saveCollectBotEvent(ctx context.Context, source g.Map, botId int64, msg *models.Message) (int64, error) {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	uniqueKey := fmt.Sprintf("bot:%d:%s:%d", botId, chatId, msg.ID)
	value, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Fields("id").Where("source_unique_key", uniqueKey).Value()
	if err != nil {
		return 0, err
	}
	if id := value.Int64(); id > 0 {
		return id, nil
	}
	rawText := strings.TrimSpace(telegramMessageText(msg))
	mediaCount, mediaJson := collectTelegramMedia(msg)
	now := gtime.Now()
	receivedAt := now
	if msg.Date > 0 {
		receivedAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	eventId, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Data(g.Map{
		"tenant_id":         source["tenant_id"],
		"account_id":        source["account_id"],
		"source_id":         source["id"],
		"source_type":       sysin.CollectSourceTypeBot,
		"bot_id":            botId,
		"source_chat_id":    chatId,
		"source_message_id": msg.ID,
		"source_grouped_id": msg.MediaGroupID,
		"source_unique_key": uniqueKey,
		"raw_text":          rawText,
		"media_count":       mediaCount,
		"media_json":        mediaJson,
		"text_hash":         collectHash(rawText),
		"dedupe_key":        collectHash(fmt.Sprintf("%s:%s:%d", rawText, mediaJson, mediaCount)),
		"status":            sysin.CollectEventStatusPending,
		"received_at":       receivedAt,
		"created_at":        now,
		"updated_at":        now,
	}).InsertAndGetId()
	if err != nil {
		return 0, err
	}
	_, _ = pdao.YoubanPublishCollectSource.Ctx(ctx).Where("id", source["id"]).Data(g.Map{
		"event_total":   gdb.Raw("event_total+1"),
		"last_event_at": now,
		"updated_at":    now,
	}).Update()
	return eventId, nil
}

func collectTelegramMedia(msg *models.Message) (int, string) {
	items := make([]map[string]string, 0, 2)
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		items = append(items, map[string]string{"type": "photo", "fileId": photo.FileID})
	}
	if msg.Video != nil {
		items = append(items, map[string]string{"type": "video", "fileId": msg.Video.FileID})
	}
	if msg.Document != nil {
		items = append(items, map[string]string{"type": "document", "fileId": msg.Document.FileID})
	}
	data, _ := json.Marshal(items)
	return len(items), string(data)
}
