package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AdminChannelBackupCreate(ctx context.Context, in *sysin.ChannelBackupCreateInp) (res *sysin.ChannelBackupCreateModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("备份频道参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, []int64{in.TgAccountId}, account.TenantId); err != nil {
		return nil, err
	}
	bots, err := s.backupChannelBots(ctx, account.TenantId, in.BotIds)
	if err != nil {
		return nil, err
	}
	channel, err := s.createTelegramBackupChannel(ctx, account.TenantId, in.TgAccountId, in.Title)
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	if err = s.upsertTgChannelCache(ctx, account.TenantId, in.TgAccountId, channel, now); err != nil {
		return nil, err
	}
	cache := backupChannelCacheModel(account.TenantId, in.TgAccountId, channel, now)
	if err = s.attachChannelBots(ctx, account.TenantId, in.TgAccountId, cache, bots); err != nil {
		return nil, err
	}
	channelId, err := s.saveBackupChannelConfig(ctx, account.Id, account.TenantId, in.TgAccountId, cache, bots)
	if err != nil {
		return nil, err
	}
	return &sysin.ChannelBackupCreateModel{
		ChannelId:    channelId,
		ChannelTitle: cache.ChannelTitle,
		TargetChatId: cache.ChannelId,
	}, nil
}

func (s *sSysPublish) backupChannelBots(ctx context.Context, tenantId int64, ids []int64) ([]*sysin.BotModel, error) {
	ids = uniqueIds(ids)
	if len(ids) > 0 {
		return s.channelCheckBots(ctx, ids, tenantId)
	}
	var bots []*sysin.BotModel
	if err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if len(bots) == 0 {
		return nil, gerror.New("请先配置可用的推送BOT")
	}
	return bots, nil
}

func (s *sSysPublish) createTelegramBackupChannel(ctx context.Context, tenantId int64, tgAccountId int64, title string) (*tg.Channel, error) {
	conf, account, err := s.backupChannelTelegram(ctx, tenantId, tgAccountId)
	if err != nil {
		return nil, err
	}
	client, err := s.backupChannelClient(ctx, conf, account)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var channel *tg.Channel
	err = s.runTelegramClientWithAccountLease(runCtx, account.Id, client, func(ctx context.Context) error {
		updates, err := client.API().ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
			Broadcast: true,
			Title:     strings.TrimSpace(title),
			About:     "小灰机聊手上架系统资料备份频道",
		})
		if err != nil {
			return gerror.Wrap(err, "创建TG备份频道失败")
		}
		channel = channelFromUpdates(updates)
		if channel == nil || channel.ID == 0 {
			return gerror.New("创建TG备份频道成功，但未读取到频道信息")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *sSysPublish) backupChannelTelegram(ctx context.Context, tenantId int64, tgAccountId int64) (*model.TelegramConfig, *sysin.TgAccountModel, error) {
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, nil, err
	}
	account, err := s.adminTgAccountById(ctx, tgAccountId, tenantId)
	if err != nil {
		return nil, nil, err
	}
	if account.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, nil, gerror.New("TG账号未授权，请先刷新账号状态或重新扫码登录")
	}
	return conf, account, nil
}

func (s *sSysPublish) backupChannelClient(ctx context.Context, conf *model.TelegramConfig, account *sysin.TgAccountModel) (*telegram.Client, error) {
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	storage, err := s.telegramSessionStorage(account.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	return telegram.NewClient(conf.AppId, conf.AppHash, options), nil
}

func channelFromUpdates(updates tg.UpdatesClass) *tg.Channel {
	var chats []tg.ChatClass
	switch data := updates.(type) {
	case *tg.Updates:
		chats = data.GetChats()
	case *tg.UpdatesCombined:
		chats = data.GetChats()
	default:
		return nil
	}
	for _, chat := range chats {
		channel, ok := chat.(*tg.Channel)
		if ok && channel.ID > 0 {
			return channel
		}
	}
	return nil
}

func backupChannelCacheModel(tenantId int64, tgAccountId int64, channel *tg.Channel, now *gtime.Time) *sysin.ChannelCacheModel {
	accessHash, _ := channel.GetAccessHash()
	username, _ := channel.GetUsername()
	return &sysin.ChannelCacheModel{
		TenantId:        tenantId,
		TgAccountId:     tgAccountId,
		ChannelId:       strconv.FormatInt(channel.ID, 10),
		AccessHash:      strconv.FormatInt(accessHash, 10),
		ChannelTitle:    channel.Title,
		ChannelUsername: strings.TrimPrefix(username, "@"),
		IsBroadcast:     boolToInt(channel.Broadcast),
		IsMegagroup:     boolToInt(channel.Megagroup),
		CanPostMessages: 1,
		CanInviteUsers:  1,
		CanAddAdmins:    1,
		LastSyncAt:      now,
	}
}

func (s *sSysPublish) saveBackupChannelConfig(ctx context.Context, operatorId int64, tenantId int64, tgAccountId int64, cache *sysin.ChannelCacheModel, bots []*sysin.BotModel) (int64, error) {
	botIds := make([]int64, 0, len(bots))
	for _, bot := range bots {
		botIds = append(botIds, bot.Id)
	}
	botJSON, err := encodeBotIds(botIds)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	existing, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		Where("target_chat_id", cache.ChannelId).
		Where("publish_direction", "backup").
		WhereNull("deleted_at").
		Value("id")
	if err != nil {
		return 0, gerror.Wrap(err, "读取备份频道配置失败")
	}
	data := g.Map{
		"tenant_id":             tenantId,
		"merchant_id":           tenantId,
		"tg_account_id":         tgAccountId,
		"channel_title":         cache.ChannelTitle,
		"channel_username":      cache.ChannelUsername,
		"target_chat_id":        cache.ChannelId,
		"publish_direction":     "backup",
		"cycle_publish_enabled": 0,
		"cycle_publish_days":    0,
		"cycle_publish_time":    "",
		"is_default_selected":   2,
		"bot_id_json":           botJSON,
		"remark":                "采集媒体备份频道",
		"status":                1,
		"updated_by":            operatorId,
		"updated_at":            now,
	}
	if existing.Int64() > 0 {
		_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", existing.Int64()).
			Data(data).
			Update()
		return existing.Int64(), gerror.Wrap(err, "更新备份频道配置失败")
	}
	data["created_by"] = operatorId
	data["created_at"] = now
	id, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	return id, gerror.Wrap(err, "保存备份频道配置失败")
}
