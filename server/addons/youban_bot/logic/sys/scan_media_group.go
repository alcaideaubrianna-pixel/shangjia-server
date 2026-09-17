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
	hglock "hotgo/internal/library/hgrds/lock"
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

func scanMediaGroupAckKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":ack"
}

func scanMediaGroupResolvedKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":resolved"
}

func scanMediaGroupLockKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":lock"
}

// acknowledgeScanMediaGroup gives immediate feedback before Telegram file
// resolution or fingerprint calculation starts. The distributed lock prevents
// concurrent album updates from sending the acknowledgement more than once.
func (s *sSysBot) acknowledgeScanMediaGroup(ctx context.Context, botId int64, userId string, msg *models.Message) error {
	if msg == nil || strings.TrimSpace(msg.MediaGroupID) == "" {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, msg.MediaGroupID))
	if err := lock.Lock(lockCtx); err != nil {
		return nil
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	ackKey := scanMediaGroupAckKey(botId, userId, msg.MediaGroupID)
	if value, err := cache.Instance().Get(ctx, ackKey); err == nil && value != nil && !value.IsNil() {
		return nil
	}
	if err := cache.Instance().Set(ctx, ackKey, 1, scanMediaGroupTTL); err != nil {
		return err
	}
	if err := s.sendMessageOnly(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "已收到，正在查询，请稍候…"); err != nil {
		_, _ = cache.Instance().Remove(ctx, ackKey)
		return err
	}
	return nil
}

func (s *sSysBot) scanMediaGroupResolved(ctx context.Context, botId int64, userId string, groupId string) bool {
	value, err := cache.Instance().Get(ctx, scanMediaGroupResolvedKey(botId, userId, groupId))
	return err == nil && value != nil && !value.IsNil()
}

func (s *sSysBot) claimScanMediaGroupResolved(ctx context.Context, botId int64, userId string, groupId string) (bool, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, groupId))
	if err := lock.Lock(lockCtx); err != nil {
		return false, nil
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	if s.scanMediaGroupResolved(ctx, botId, userId, groupId) {
		return false, nil
	}
	if err := cache.Instance().Set(ctx, scanMediaGroupResolvedKey(botId, userId, groupId), 1, scanMediaGroupTTL); err != nil {
		return false, err
	}
	return true, nil
}

func (s *sSysBot) finishScanMediaGroup(botId int64, userId string, groupId string) {
	time.Sleep(1500 * time.Millisecond)
	ctx := context.Background()
	key := scanMediaGroupKey(botId, userId, groupId)
	if s.scanMediaGroupResolved(ctx, botId, userId, groupId) {
		_, _ = cache.Instance().Remove(ctx, key)
		return
	}
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
		observeScanRequest(ctx, botId, "failed")
		_ = s.replyBotError(ctx, botId, pending.ChatId, "扫图搜索", err)
		return
	}
	startedAt := time.Now()
	if err = s.searchScanMediaAndReply(ctx, botId, pending.ChatId, account, pending.Items); err != nil {
		observeScanStage(ctx, botId, "group_total", startedAt, err)
		observeScanRequest(ctx, botId, "failed")
		g.Log().Warningf(ctx, "处理扫图媒体组失败 botId:%d userId:%s err:%+v", botId, userId, err)
		return
	}
	observeScanStage(ctx, botId, "group_total", startedAt, nil)
	observeScanRequest(ctx, botId, "fingerprint")
}

func parseTelegramUserId(value string) int64 {
	var id int64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &id)
	return id
}
