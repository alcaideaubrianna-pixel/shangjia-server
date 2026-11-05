package sys

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"strings"
)

func (s *sSysPublish) BotChannelMembershipChanged(ctx context.Context, botId int64, chatId, status, role string, updateId int64) error {
	rawChatID := strings.TrimSpace(chatId)
	chatId = normalizeTelegramChannelChatID(chatId)
	g.Log().Infof(ctx, "Telegram BOT频道成员变更 bot_id:%d update_id:%d raw_chat_id:%q normalized_chat_id:%q new_status:%q role:%q", botId, updateId, rawChatID, chatId, status, role)
	if botId <= 0 || strings.TrimSpace(chatId) == "" {
		g.Log().Warningf(ctx, "忽略无效Telegram BOT频道成员变更 bot_id:%d update_id:%d raw_chat_id:%q", botId, updateId, rawChatID)
		return nil
	}
	var bot struct {
		TenantId int64 `json:"tenant_id"`
	}
	if err := g.DB().Model(publishBotTable).Ctx(ctx).Fields("tenant_id").Where("id", botId).WhereNull("deleted_at").Scan(&bot); err != nil || bot.TenantId <= 0 {
		return err
	}
	now := gtime.Now()
	var channel struct {
		Id int64 `json:"id"`
	}
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id").Where("tenant_id", bot.TenantId).WhereIn("telegram_chat_id", tgChannelCacheLookupIds(chatId)).Scan(&channel); err != nil {
		return err
	}
	if channel.Id <= 0 {
		g.Log().Warningf(ctx, "Telegram BOT频道成员变更未找到频道目录，创建占位记录 bot_id:%d update_id:%d chat_id:%q status:%q role:%q", botId, updateId, chatId, status, role)
		if err := s.upsertCollectChannel(ctx, bot.TenantId, chatId, "", chatId, "", 0, 0, 0, now, now); err != nil {
			return err
		}
		if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id").Where("tenant_id", bot.TenantId).Where("telegram_chat_id", chatId).Scan(&channel); err != nil || channel.Id <= 0 {
			return err
		}
	}
	state := "discovered"
	removed := status == "left" || status == "kicked"
	if removed {
		state = "bot_removed"
	}
	data := pdo.YoubanPublishCollectBotChannel{TenantId: bot.TenantId, BotId: botId, ChannelId: channel.Id, Status: state, BotRole: role, LastSeenAt: now, LastUpdateId: updateId, RemovedReason: gdbString(removed, status)}
	if removed {
		data.RemovedAt = now
	}
	_, err := pdao.YoubanPublishCollectBotChannel.Ctx(ctx).Data(data).OnConflict("tenant_id,bot_id,channel_id").OnDuplicate("status,bot_role,last_seen_at,last_update_id,updated_at,removed_at,removed_reason").Save()
	if err != nil {
		return err
	}
	s.refreshCollectSourceCache(ctx)
	if removed {
		var title struct {
			Value string `json:"title"`
		}
		_ = pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("title AS value").Where("id", channel.Id).Scan(&title)
		_ = botService.SysBot().NotifySuperAdmins(ctx, botId, "collect_source", "BOT 已被移出采集频道："+title.Value+"（"+chatId+"），该频道已暂停采集。")
	}
	return nil
}

func gdbNil(removed bool) interface{} {
	if removed {
		return gtime.Now()
	}
	return nil
}
func gdbString(removed bool, value string) string {
	if removed {
		return value
	}
	return ""
}
