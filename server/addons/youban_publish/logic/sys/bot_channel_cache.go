package sys

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const publishBotChannelCacheTable = "hg_youban_publish_bot_channel_cache"

func (s *sSysPublish) cacheBotMessage(ctx context.Context, tenantId, botId int64, msg *models.Message) error {
	if tenantId <= 0 || botId <= 0 || msg == nil || msg.Chat.ID == 0 {
		return nil
	}
	chatType := strings.TrimSpace(string(msg.Chat.Type))
	if chatType != "private" && chatType != "channel" && chatType != "group" && chatType != "supergroup" {
		return nil
	}
	chatId := g.NewVar(msg.Chat.ID).String()
	title := strings.TrimSpace(msg.Chat.Title)
	if title == "" {
		title = strings.TrimSpace(msg.Chat.Username)
	}
	isBroadcast, isMegagroup := 0, 0
	if chatType == "channel" {
		isBroadcast = 1
	}
	if chatType == "group" || chatType == "supergroup" {
		isMegagroup = 1
	}
	now := gtime.Now()
	lastAt := now
	if msg.Date > 0 {
		lastAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	data := g.Map{
		"tenant_id": tenantId, "bot_id": botId, "chat_id": chatId, "chat_type": chatType,
		"chat_title": title, "chat_username": strings.TrimSpace(msg.Chat.Username),
		"is_broadcast": isBroadcast, "is_megagroup": isMegagroup,
		"last_message_text": strings.TrimSpace(telegramMessageText(msg)), "last_message_at": lastAt, "updated_at": now,
	}
	var exists struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishBotChannelCacheTable).Safe().Ctx(ctx).Fields("id").Where("tenant_id", tenantId).Where("bot_id", botId).Where("chat_id", chatId).Scan(&exists); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if exists.Id > 0 {
		_, err := g.DB().Model(publishBotChannelCacheTable).Safe().Ctx(ctx).Where("id", exists.Id).Data(data).Increment("message_count", 1)
		return err
	}
	data["message_count"] = 1
	data["created_at"] = now
	_, err := g.DB().Model(publishBotChannelCacheTable).Safe().Ctx(ctx).Data(data).Insert()
	return err
}

func (s *sSysPublish) AdminBotChannelCacheList(ctx context.Context, in *sysin.BotChannelCacheListInp) (list []*sysin.BotChannelCacheModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.BotChannelCacheListInp{}
	}
	mod := g.DB().Model(publishBotChannelCacheTable+" c").Safe().Ctx(ctx).
		LeftJoin(publishBotTable+" b", "b.id=c.bot_id").
		Where("c.tenant_id", account.TenantId).WhereNull("b.deleted_at").
		Fields("c.id,c.bot_id,b.bot_username,c.chat_id AS channel_id,c.chat_title AS channel_title,c.chat_username AS channel_username,c.chat_type,CASE WHEN c.chat_type='private' THEN 1 ELSE 0 END AS is_private,c.is_broadcast,c.is_megagroup,c.message_count,c.last_message_text,c.last_message_at,c.created_at,c.updated_at")
	if in.BotId > 0 {
		mod = mod.Where("c.bot_id", in.BotId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(c.chat_id LIKE ? OR c.chat_title LIKE ? OR c.chat_username LIKE ? OR b.bot_username LIKE ?)", like, like, like, like)
	}
	switch strings.ToLower(strings.TrimSpace(in.Type)) {
	case "private":
		mod = mod.Where("c.chat_type", "private")
	case "channel":
		mod = mod.Where("c.is_broadcast", 1)
	case "group":
		mod = mod.Where("c.is_megagroup", 1)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("c.last_message_at").OrderDesc("c.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot频道缓存失败")
	}
	return list, totalCount, nil
}

var _ gdb.Record
