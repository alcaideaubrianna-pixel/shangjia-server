package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/internal/library/cache"
)

const listenerSenderCacheTTL = 24 * time.Hour

type listenerSenderRecord struct {
	Id                int64       `json:"id"`
	TenantId          int64       `json:"tenantId"`
	TgAccountId       int64       `json:"tgAccountId"`
	TelegramUserId    string      `json:"telegramUserId"`
	TelegramUsername  string      `json:"telegramUsername"`
	TelegramFirstName string      `json:"telegramFirstName"`
	TelegramLastName  string      `json:"telegramLastName"`
	DisplayName       string      `json:"displayName"`
	LastSeenAt        *gtime.Time `json:"lastSeenAt"`
}

func (s *sSysPublish) resolveListenerSender(ctx context.Context, plan accountListenPlanRuntime, sourceChatId string, msg *tg.Message, sender listenerMessageSenderInfo) listenerMessageSenderInfo {
	sender = normalizeListenerSender(sender)
	if sender.UserId == "" || listenerSenderComplete(sender) {
		_ = s.saveListenerSender(ctx, plan, sender)
		return sender
	}
	if cached, ok := s.listenerSenderFromCache(ctx, plan.TgAccountId, sender.UserId); ok {
		return cached
	}
	if stored, ok := s.listenerSenderFromDB(ctx, plan.TgAccountId, sender.UserId); ok {
		_ = s.cacheListenerSender(ctx, plan.TgAccountId, stored)
		return stored
	}
	fetched, err := s.fetchListenerSenderFromTelegram(ctx, plan, sourceChatId, msg, sender.UserId)
	if err != nil {
		g.Log().Debugf(ctx, "补全监听发送者资料失败 tgAccountId:%d user:%s msg:%d err:%+v", plan.TgAccountId, sender.UserId, msg.ID, err)
		return sender
	}
	if fetched.UserId == "" {
		return sender
	}
	_ = s.saveListenerSender(ctx, plan, fetched)
	return fetched
}

func (s *sSysPublish) fetchListenerSenderFromTelegram(ctx context.Context, plan accountListenPlanRuntime, sourceChatId string, msg *tg.Message, userId string) (listenerMessageSenderInfo, error) {
	if msg == nil || strings.TrimSpace(userId) == "" {
		return listenerMessageSenderInfo{}, nil
	}
	uid, err := strconv.ParseInt(strings.TrimSpace(userId), 10, 64)
	if err != nil || uid <= 0 {
		return listenerMessageSenderInfo{}, nil
	}
	var out listenerMessageSenderInfo
	usedRuntime, err := s.executeAccountCollectOperation(ctx, plan.TgAccountId, 30*time.Second, func(runCtx context.Context, client *telegram.Client) error {
		sender, fetchErr := s.fetchListenerSenderWithClient(runCtx, plan, sourceChatId, msg, uid, client)
		if fetchErr != nil {
			return fetchErr
		}
		out = sender
		return nil
	})
	if err != nil {
		return listenerMessageSenderInfo{}, err
	}
	if usedRuntime {
		return out, nil
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return listenerMessageSenderInfo{}, err
	}
	account, err := s.accountCollectTgAccount(ctx, plan.TgAccountId)
	if err != nil {
		return listenerMessageSenderInfo{}, err
	}
	client, err := s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
	if err != nil {
		return listenerMessageSenderInfo{}, err
	}
	err = s.runTelegramClientWithAccountLease(ctx, plan.TgAccountId, client, func(runCtx context.Context) error {
		if _, err := client.Self(runCtx); err != nil {
			return err
		}
		out, err = s.fetchListenerSenderWithClient(runCtx, plan, sourceChatId, msg, uid, client)
		return err
	})
	return out, err
}

func (s *sSysPublish) fetchListenerSenderWithClient(ctx context.Context, plan accountListenPlanRuntime, sourceChatId string, msg *tg.Message, userId int64, client *telegram.Client) (listenerMessageSenderInfo, error) {
	if client == nil || msg == nil {
		return listenerMessageSenderInfo{}, nil
	}
	peer, err := s.listenerSourceInputPeer(ctx, plan, sourceChatId, msg, client)
	if err != nil {
		return listenerMessageSenderInfo{}, err
	}
	users, err := client.API().UsersGetUsers(ctx, []tg.InputUserClass{
		&tg.InputUserFromMessage{
			Peer:   peer,
			MsgID:  msg.ID,
			UserID: userId,
		},
	})
	if err != nil {
		return listenerMessageSenderInfo{}, gerror.Wrap(err, "获取TG发送者资料失败")
	}
	for _, item := range users {
		user, ok := item.(*tg.User)
		if !ok || user.ID != userId {
			continue
		}
		return listenerSenderFromUser(user), nil
	}
	return listenerMessageSenderInfo{}, nil
}

func (s *sSysPublish) listenerSourceInputPeer(ctx context.Context, plan accountListenPlanRuntime, sourceChatId string, msg *tg.Message, client *telegram.Client) (tg.InputPeerClass, error) {
	if msg != nil && msg.PeerID != nil {
		switch peer := msg.PeerID.(type) {
		case *tg.PeerChat:
			return &tg.InputPeerChat{ChatID: peer.ChatID}, nil
		case *tg.PeerChannel:
			if sourceChatId == "" {
				sourceChatId = normalizeTelegramChannelChatID(strconv.FormatInt(peer.ChannelID, 10))
			}
		}
	}
	if peer, err := s.collectMediaInputPeer(ctx, plan.TenantId, plan.TgAccountId, client, sourceChatId); err == nil {
		return peer, nil
	}
	return nil, gerror.New("无法解析监听来源会话")
}

func (s *sSysPublish) listenerSenderFromCache(ctx context.Context, tgAccountId int64, userId string) (listenerMessageSenderInfo, bool) {
	var sender listenerMessageSenderInfo
	value, err := cache.Instance().Get(ctx, listenerSenderCacheKey(tgAccountId, userId))
	if err != nil || value.IsNil() {
		return sender, false
	}
	if err = value.Scan(&sender); err != nil {
		return listenerMessageSenderInfo{}, false
	}
	sender = normalizeListenerSender(sender)
	return sender, listenerSenderComplete(sender)
}

func (s *sSysPublish) cacheListenerSender(ctx context.Context, tgAccountId int64, sender listenerMessageSenderInfo) error {
	sender = normalizeListenerSender(sender)
	if sender.UserId == "" {
		return nil
	}
	return cache.Instance().Set(ctx, listenerSenderCacheKey(tgAccountId, sender.UserId), sender, listenerSenderCacheTTL)
}

func (s *sSysPublish) listenerSenderFromDB(ctx context.Context, tgAccountId int64, userId string) (listenerMessageSenderInfo, bool) {
	var row *listenerSenderRecord
	err := g.DB().Model(messageListenSenderTable).Safe().Ctx(ctx).
		Where("tg_account_id", tgAccountId).
		Where("telegram_user_id", strings.TrimSpace(userId)).
		Scan(&row)
	if err != nil || row == nil {
		return listenerMessageSenderInfo{}, false
	}
	sender := normalizeListenerSender(listenerMessageSenderInfo{
		UserId:      row.TelegramUserId,
		Username:    row.TelegramUsername,
		DisplayName: row.DisplayName,
	})
	if sender.DisplayName == "" {
		sender.DisplayName = strings.TrimSpace(strings.Join([]string{strings.TrimSpace(row.TelegramFirstName), strings.TrimSpace(row.TelegramLastName)}, " "))
	}
	return sender, listenerSenderComplete(sender)
}

func (s *sSysPublish) saveListenerSender(ctx context.Context, plan accountListenPlanRuntime, sender listenerMessageSenderInfo) error {
	sender = normalizeListenerSender(sender)
	if sender.UserId == "" || !listenerSenderComplete(sender) {
		return nil
	}
	now := gtime.Now()
	firstName, lastName := splitListenerDisplayName(sender.DisplayName)
	data := g.Map{
		"tenant_id":           plan.TenantId,
		"tg_account_id":       plan.TgAccountId,
		"telegram_user_id":    sender.UserId,
		"telegram_username":   sender.Username,
		"telegram_first_name": firstName,
		"telegram_last_name":  lastName,
		"display_name":        sender.DisplayName,
		"last_seen_at":        now,
		"updated_at":          now,
	}
	result, err := g.DB().Model(messageListenSenderTable).Safe().Ctx(ctx).
		Where("tg_account_id", plan.TgAccountId).
		Where("telegram_user_id", sender.UserId).
		Data(data).
		Update()
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		data["created_at"] = now
		if _, err = g.DB().Model(messageListenSenderTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil && !isDuplicateKeyError(err) {
			return err
		}
	}
	return s.cacheListenerSender(ctx, plan.TgAccountId, sender)
}

func normalizeListenerSender(sender listenerMessageSenderInfo) listenerMessageSenderInfo {
	sender.UserId = strings.TrimSpace(sender.UserId)
	sender.Username = strings.TrimSpace(strings.TrimPrefix(sender.Username, "@"))
	sender.DisplayName = strings.TrimSpace(sender.DisplayName)
	return sender
}

func listenerSenderComplete(sender listenerMessageSenderInfo) bool {
	sender = normalizeListenerSender(sender)
	return sender.UserId != "" && (sender.Username != "" || sender.DisplayName != "")
}

func listenerSenderCacheKey(tgAccountId int64, userId string) string {
	return fmt.Sprintf("youban_publish:message_listen:sender:%d:%s", tgAccountId, strings.TrimSpace(userId))
}

func splitListenerDisplayName(name string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
