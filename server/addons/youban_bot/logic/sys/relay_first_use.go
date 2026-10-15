package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/os/gtime"
)

// Use the durable outbox uniqueness constraint. Each binding endpoint notifies
// each currently confirmed administrator once, including across replicas/restarts.
func (s *sSysBot) enqueueRelayFirstUse(ctx context.Context, cfg *entity.YoubanBotRelayConfig, msg *models.Message, binding *entity.YoubanBotRelayBinding) error {
	if msg.Chat.Type != models.ChatTypePrivate || msg.From == nil || msg.From.IsBot {
		return nil
	}
	var admins []*entity.YoubanBotRelayAdmin
	if err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(dao.YoubanBotRelayAdmin.Columns().BotId, cfg.BotId).Scan(&admins); err != nil {
		return err
	}
	recipients := map[string]bool{}
	for _, a := range admins {
		if id, err := strconv.ParseInt(a.UserId, 10, 64); err == nil && id > 0 {
			recipients[a.UserId] = true
		}
	}
	owners, err := s.relayOwnerTelegramIDs(ctx, cfg)
	if err != nil {
		return err
	}
	for _, id := range owners {
		recipients[id] = true
	}
	source := strconv.FormatInt(msg.Chat.ID, 10)
	name := firstNonEmpty(strings.TrimSpace(msg.From.FirstName+" "+msg.From.LastName), source)
	username := ""
	if msg.From.Username != "" {
		username = " @" + msg.From.Username
	}
	text := fmt.Sprintf("<b>绑定用户开始使用</b>\nBot ID：<code>%d</code>\n用户：%s%s\n用户ID：<code>%s</code>\n绑定：%s ↔ %s\n该用户已发送绑定后的首条普通消息。", cfg.BotId, html.EscapeString(name), html.EscapeString(username), source, html.EscapeString(binding.LeftName), html.EscapeString(binding.RightName))
	raw, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return err
	}
	for recipient := range recipients {
		if recipient == source {
			continue
		}
		data := do.YoubanBotRelayDelivery{BotId: cfg.BotId, BindingId: binding.Id, SourceChatId: source, SourceMessageId: -binding.Id, TargetChatId: recipient, Kind: consts.RelayKindNotice, PairKey: binding.PairKey, RawJson: string(raw), Status: consts.RelayPending, AvailableAt: gtime.Now()}
		c := dao.YoubanBotRelayDelivery.Columns()
		if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(data).OnConflict(c.BotId, c.SourceChatId, c.SourceMessageId, c.Kind, c.TargetChatId).InsertIgnore(); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysBot) enqueueRelayBindingActivated(ctx context.Context, cfg *entity.YoubanBotRelayConfig, msg *models.Message, binding *entity.YoubanBotRelayBinding) error {
	if msg.From == nil || binding == nil {
		return nil
	}
	var admins []*entity.YoubanBotRelayAdmin
	if err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(dao.YoubanBotRelayAdmin.Columns().BotId, cfg.BotId).Scan(&admins); err != nil {
		return err
	}
	recipients := map[string]bool{}
	for _, admin := range admins {
		if id, err := strconv.ParseInt(admin.UserId, 10, 64); err == nil && id > 0 {
			recipients[admin.UserId] = true
		}
	}
	owners, err := s.relayOwnerTelegramIDs(ctx, cfg)
	if err != nil {
		return err
	}
	for _, id := range owners {
		recipients[id] = true
	}
	source := strconv.FormatInt(msg.From.ID, 10)
	name := firstNonEmpty(strings.TrimSpace(msg.From.FirstName+" "+msg.From.LastName), "@"+msg.From.Username, source)
	text := fmt.Sprintf("<b>待确认绑定已生效</b>\nBot ID：<code>%d</code>\n用户：%s @%s\n用户ID：<code>%s</code>\n绑定：%s ↔ %s\n用户首次使用机器人，系统已自动确认绑定。", cfg.BotId, html.EscapeString(name), html.EscapeString(msg.From.Username), source, html.EscapeString(binding.LeftName), html.EscapeString(binding.RightName))
	raw, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return err
	}
	for recipient := range recipients {
		if recipient == source {
			continue
		}
		data := do.YoubanBotRelayDelivery{BotId: cfg.BotId, BindingId: binding.Id, SourceChatId: source, SourceMessageId: -1000000000 - binding.Id, TargetChatId: recipient, Kind: consts.RelayKindNotice, PairKey: binding.PairKey, RawJson: string(raw), Status: consts.RelayPending, AvailableAt: gtime.Now()}
		c := dao.YoubanBotRelayDelivery.Columns()
		if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(data).OnConflict(c.BotId, c.SourceChatId, c.SourceMessageId, c.Kind, c.TargetChatId).InsertIgnore(); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysBot) sendRelayFirstUse(ctx context.Context, bot *tgbot.Bot, cfg *entity.YoubanBotRelayConfig, row *entity.YoubanBotRelayDelivery) ([]int, string, error) {
	count, err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{Id: row.BindingId, BotId: cfg.BotId}).Count()
	if err != nil {
		return nil, relayRetryStatus(row.Attempts), err
	}
	if count == 0 {
		return nil, consts.RelayCancelled, nil
	}
	id, err := strconv.ParseInt(row.TargetChatId, 10, 64)
	if err != nil {
		return nil, consts.RelayCancelled, err
	}
	allowed, err := s.relayIsAdmin(ctx, cfg, id)
	if err != nil {
		return nil, relayRetryStatus(row.Attempts), err
	}
	if !allowed {
		return nil, consts.RelayCancelled, nil
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(row.RawJson), &payload); err != nil {
		return nil, consts.RelayFailed, err
	}
	message, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: row.TargetChatId, Text: payload.Text, ParseMode: models.ParseModeHTML})
	if err != nil {
		return nil, relaySendErrorStatus(err, row.Attempts), err
	}
	return []int{message.ID}, consts.RelaySent, nil
}
