package sys

import (
	"context"
	"errors"
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

// Channel discovery precedes collection and does not create collection input.
// In particular, membership updates must never become empty profile events.
type collectChannelUpdateFeature struct{}

func init() {
	gatewayservice.RegisterFeature(&collectChannelUpdateFeature{})
}

func (*collectChannelUpdateFeature) Key() string   { return "youban_publish_channel_discovery" }
func (*collectChannelUpdateFeature) Priority() int { return 1000 }
func (*collectChannelUpdateFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

type collectChannelUpdate struct {
	Chat          models.Chat
	MessageID     int64
	Membership    *models.ChatMemberUpdated
	SelfMember    bool
	DiscoveryOnly bool
}

func collectChannelUpdateFromWebhook(update *models.Update) (collectChannelUpdate, bool) {
	if update == nil {
		return collectChannelUpdate{}, false
	}
	var result collectChannelUpdate
	switch {
	case update.MyChatMember != nil:
		result.Chat, result.Membership, result.SelfMember = update.MyChatMember.Chat, update.MyChatMember, true
	case update.ChatMember != nil:
		result.Chat, result.Membership = update.ChatMember.Chat, update.ChatMember
	case update.ChatJoinRequest != nil:
		result.Chat, result.DiscoveryOnly = update.ChatJoinRequest.Chat, true
	case update.MessageReaction != nil:
		result.Chat, result.DiscoveryOnly = update.MessageReaction.Chat, true
	case update.MessageReactionCount != nil:
		result.Chat, result.DiscoveryOnly = update.MessageReactionCount.Chat, true
	case update.ChatBoost != nil:
		result.Chat, result.DiscoveryOnly = update.ChatBoost.Chat, true
	case update.RemovedChatBoost != nil:
		result.Chat, result.DiscoveryOnly = update.RemovedChatBoost.Chat, true
	case update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil:
		result.Chat = update.CallbackQuery.Message.Message.Chat
	case update.CallbackQuery != nil && update.CallbackQuery.Message.InaccessibleMessage != nil:
		result.Chat = update.CallbackQuery.Message.InaccessibleMessage.Chat
	default:
		message, _ := telegramUpdateMessage(update)
		if message == nil {
			return result, false
		}
		result.Chat, result.MessageID = message.Chat, int64(message.ID)
		result.DiscoveryOnly = len(message.NewChatMembers) > 0 || message.LeftChatMember != nil
	}
	if result.Chat.ID == 0 {
		return result, false
	}
	switch result.Chat.Type {
	case models.ChatTypeChannel, models.ChatTypeGroup, models.ChatTypeSupergroup:
		return result, true
	default:
		return result, false
	}
}

func (*collectChannelUpdateFeature) HandleUpdate(ctx context.Context, bot gatewayservice.BotContext, update *models.Update) (bool, error) {
	event, ok := collectChannelUpdateFromWebhook(update)
	if !ok {
		return false, nil
	}
	// Ordinary and edited messages already invoke the shared binding function
	// in collector delivery. Do not add a second source query on that hot path.
	if event.Membership == nil && !event.DiscoveryOnly && update.CallbackQuery == nil {
		return false, nil
	}
	s := &sSysPublish{}
	seen := make(map[[2]int64]struct{})
	var failures []error
	handled := false
	for _, binding := range bot.Bindings {
		if binding.Owner != "youban_publish" || binding.TenantID <= 0 || binding.ReferenceID <= 0 {
			continue
		}
		key := [2]int64{binding.TenantID, binding.ReferenceID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if event.Membership != nil || event.DiscoveryOnly {
			handled = true
		}
		if err := s.discoverCollectChannelUpdate(ctx, binding, update.ID, event); err != nil {
			failures = append(failures, err)
		}
	}
	return handled, errors.Join(failures...)
}

func (s *sSysPublish) discoverCollectChannelUpdate(ctx context.Context, binding gatewayservice.BotBinding, updateID int64, event collectChannelUpdate) error {
	tenantID, botID := binding.TenantID, binding.ReferenceID
	chatID := strconv.FormatInt(event.Chat.ID, 10)
	if event.SelfMember {
		// Use the Chat already supplied by Telegram rather than calling getChat.
		broadcast, megagroup := 0, 0
		if event.Chat.Type == models.ChatTypeChannel {
			broadcast = 1
		} else if event.Chat.Type == models.ChatTypeSupergroup {
			megagroup = 1
		}
		if err := s.upsertCollectChannel(ctx, tenantID, chatID, string(event.Chat.Type), event.Chat.Title, event.Chat.Username, broadcast, megagroup, 0, gtime.Now(), gtime.Now()); err != nil {
			return gerror.Wrap(err, "登记BOT成员事件频道失败")
		}
		status := string(event.Membership.NewChatMember.Type)
		if err := s.BotChannelMembershipChanged(ctx, botID, chatID, status, status, updateID); err != nil {
			return err
		}
		if event.Membership.NewChatMember.Type == models.ChatMemberTypeLeft || event.Membership.NewChatMember.Type == models.ChatMemberTypeBanned {
			return nil
		}
	} else {
		// A reaction, callback or another user's role change must not revive an
		// explicitly removed BOT. Missing discovery records may be created.
		cols := pdao.YoubanPublishCollectBotChannel.Columns()
		channelCols := pdao.YoubanPublishCollectChannel.Columns()
		row, err := pdao.YoubanPublishCollectBotChannel.Ctx(ctx).
			Where(cols.TenantId, tenantID).Where(cols.BotId, botID).
			WhereNull(cols.DeletedAt).
			WhereIn(cols.ChannelId, pdao.YoubanPublishCollectChannel.Ctx(ctx).
				Fields(channelCols.Id).Where(channelCols.TenantId, tenantID).
				WhereIn(channelCols.TelegramChatId, tgChannelCacheLookupIds(chatID)).
				WhereNull(channelCols.DeletedAt)).OrderDesc(cols.LastUpdateId).OrderDesc(cols.UpdatedAt).Limit(1).One()
		if err != nil {
			return gerror.Wrap(err, "读取BOT成员事件频道归属失败")
		}
		if !row.IsEmpty() && row[cols.Status].String() == sysin.CollectBotChannelRemoved {
			return nil
		}
	}
	cols := pdao.YoubanPublishCollectSource.Columns()
	sources, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields(cols.Id, cols.TenantId, cols.AccountId, cols.BotId, cols.BotCollectScope).
		Where(cols.TenantId, tenantID).Where(cols.BotId, botID).
		Where(cols.SourceType, sysin.CollectSourceTypeBot).
		Where(cols.Status, 1).Where(cols.CollectEnabled, 1).WhereNull(cols.DeletedAt).All()
	if err != nil {
		return gerror.Wrap(err, "读取频道发现的BOT采集源失败")
	}
	g.Log().Info(ctx, "采集频道发现已匹配来源", g.Map{
		"event": "collect.channel.discovery.sources", "tenantId": tenantID,
		"botId": botID, "chatId": chatID, "updateId": updateID, "sourceCount": len(sources),
	})
	var failures []error
	for _, source := range sources {
		allowed, bindErr := s.ensureBotCollectChannelApproval(ctx, source, &collectorin.CollectorDelivery{
			TenantID: tenantID, SourceID: source["id"].Int64(), SourceChatID: chatID, SourceMessageID: event.MessageID,
		})
		if bindErr != nil {
			failures = append(failures, gerror.Wrapf(bindErr, "绑定BOT采集频道失败 sourceId:%d", source["id"].Int64()))
		}
		if event.Membership != nil {
			g.Log().Info(ctx, "采集频道成员事件绑定完成", g.Map{
				"event": "collect.channel.membership.binding", "tenantId": tenantID,
				"botId": botID, "sourceId": source["id"].Int64(), "accountId": source["account_id"].Int64(),
				"chatId": chatID, "updateId": updateID, "allowed": allowed, "error": bindErr,
			})
		}
	}
	return errors.Join(failures...)
}
