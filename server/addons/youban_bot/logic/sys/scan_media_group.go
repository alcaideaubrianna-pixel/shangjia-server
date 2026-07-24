package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const scanMediaGroupTTL = 2 * time.Minute

type scanMediaGroupPending struct {
	BotId     int64                       `json:"botId"`
	ChatId    string                      `json:"chatId"`
	UserId    string                      `json:"userId"`
	CreatedAt int64                       `json:"createdAt"`
	Items     []*sysin.BotMediaSearchItem `json:"items"`
}

func (s *sSysBot) collectScanMediaGroup(ctx context.Context, botId int64, userId string, msg *models.Message, items []*sysin.BotMediaSearchItem) error {
	if msg == nil {
		return nil
	}
	key := scanMediaGroupKey(botId, userId, msg.MediaGroupID)
	pending := &scanMediaGroupPending{}
	value, _ := cache.Instance().Get(ctx, key)
	isFirst := true
	if value != nil && !value.IsNil() {
		if err := json.Unmarshal([]byte(value.String()), pending); err == nil && pending.BotId == botId {
			isFirst = false
		}
	}
	if isFirst {
		pending = &scanMediaGroupPending{BotId: botId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), UserId: userId, CreatedAt: time.Now().Unix()}
	}
	pending.Items = append(pending.Items, items...)
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	if err = cache.Instance().Set(ctx, key, string(data), scanMediaGroupTTL); err != nil {
		return err
	}
	if isFirst {
		go s.finishScanMediaGroup(botId, userId, msg.MediaGroupID)
	}
	return nil
}

func scanMediaGroupKey(botId int64, userId string, groupId string) string {
	return fmt.Sprintf("youban_bot:scan_group:%d:%s:%s", botId, strings.TrimSpace(userId), strings.TrimSpace(groupId))
}

func (s *sSysBot) finishScanMediaGroup(botId int64, userId string, groupId string) {
	time.Sleep(1500 * time.Millisecond)
	ctx := context.Background()
	key := scanMediaGroupKey(botId, userId, groupId)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value == nil || value.IsNil() {
		return
	}
	var pending scanMediaGroupPending
	if err = json.Unmarshal([]byte(value.String()), &pending); err != nil || len(pending.Items) == 0 {
		return
	}
	_, _ = cache.Instance().Remove(ctx, key)
	account, err := s.boundProfileAccountByUser(ctx, parseTelegramUserId(userId))
	if err != nil {
		_ = s.replyBotError(ctx, botId, pending.ChatId, "扫图搜索", err)
		return
	}
	if err = s.searchScanMediaAndReply(ctx, botId, pending.ChatId, account, pending.Items); err != nil {
		g.Log().Warningf(ctx, "处理扫图媒体组失败 botId:%d userId:%s err:%+v", botId, userId, err)
	}
}

func parseTelegramUserId(value string) int64 {
	var id int64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &id)
	return id
}
