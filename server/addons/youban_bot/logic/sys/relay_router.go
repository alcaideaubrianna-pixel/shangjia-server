package sys

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysBot) relayPluginEnabled(ctx context.Context) (bool, error) {
	// Read shared state so disabling on another API instance takes effect immediately.
	var feature *botFeatureRow
	err := dao.YoubanBotFeature.Ctx(ctx).Where(dao.YoubanBotFeature.Columns().FeatureKey, consts.RelayFeatureKey).Scan(&feature)
	return feature != nil && feature.Status == 1, gerror.Wrap(err, "读取双向机器人插件状态失败")
}

func (s *sSysBot) handleRelayUpdate(ctx context.Context, botID int64, update *models.Update) (bool, error) {
	if update == nil || update.Message == nil {
		return false, nil
	}
	msg := update.Message
	if msg.Chat.ID == 0 || (msg.From == nil && msg.SenderChat == nil) || (msg.From != nil && msg.From.IsBot && msg.SenderChat == nil) {
		return false, nil
	}
	if msg.Chat.Type != models.ChatTypePrivate && msg.Chat.Type != models.ChatTypeGroup && msg.Chat.Type != models.ChatTypeSupergroup {
		return false, nil
	}
	cfg, err := s.relayIdentity(ctx, botID)
	if err != nil {
		g.Log().Warningf(ctx, "双向功能配置不可用，保留原Bot处理 botId:%d err:%+v", botID, err)
		return false, nil
	}
	if cfg == nil {
		return false, nil
	}
	if !relayOwned(cfg) {
		return true, s.relayReply(ctx, botID, msg, "此双向机器人尚未完成租户账户归属迁移，已停用。请联系管理员核对后台Bot归属后迁移；不会继续传递消息。")
	}
	row, err := s.botById(ctx, botID)
	if err != nil {
		return false, err
	}
	if row.Status != 1 {
		return false, nil
	}
	source := strconv.FormatInt(msg.Chat.ID, 10)
	if source == cfg.AuditGroupId {
		return true, nil
	} // Audit traffic must never re-enter business routing.
	if err = s.relayObserveChat(ctx, botID, msg); err != nil {
		return true, err
	}
	if err = s.relayResolvePendingAdmins(ctx, botID, msg); err != nil {
		return true, err
	}
	command, _ := botCommandAndArgs(msg.Text)
	if command == "start" {
		return true, s.relayWelcome(ctx, cfg, msg)
	}
	if s.relayMainMenuInput(ctx, msg.Text) {
		return true, s.relayReply(ctx, botID, msg, "这里是双向聊天机器人。管理请到主机器人发送 /relayadmin；聊天帮助请联系管理员。")
	}
	active, state, err := s.relayRuntimeState(ctx, cfg)
	if err != nil {
		return true, err
	}
	parsed := parseRelayCommand(msg.Text)
	if !active && (parsed.Name == "/help" || parsed.Name == "/id") {
		return true, s.handleRelayCommand(ctx, cfg, msg, parsed)
	}
	if !active {
		if msg.Chat.Type != models.ChatTypePrivate && parsed.Name == "" {
			return true, nil
		}
		return true, s.relayReply(ctx, botID, msg, "当前双向聊天不可用："+state+"。请联系管理员。")
	}
	binding, err := relayBindingByChat(ctx, botID, source)
	if err != nil {
		return true, err
	}
	cmd := parseRelayCommand(msg.Text)
	if cmd.Name == "" && strings.TrimSpace(msg.Text) == (relayFeature{}).Description() {
		cmd = relayCommand{Name: "/help"}
	}
	// Telegram service messages are not copyable business content.
	if !relayMessageHasContent(msg) {
		return true, nil
	}
	target := ""
	if binding != nil {
		target = binding.LeftChatId
		if source == target {
			target = binding.RightChatId
		}
	}
	var rules []*entity.YoubanBotRelayRule
	if err = dao.YoubanBotRelayRule.Ctx(ctx).Where(dao.YoubanBotRelayRule.Columns().BotId, botID).Scan(&rules); err != nil {
		return true, gerror.Wrap(err, "读取消息屏蔽规则失败")
	}
	reason := ""
	if binding != nil && cmd.Name == "" {
		reason = relayFilterReason(cfg, rules, msg, target)
	}
	// Archive all observed human messages, including filtered messages and commands;
	// neither audit nor archive failures are silently acknowledged.
	if err = s.enqueueRelayMessage(ctx, cfg, row.BotName, msg, binding, target, reason, cmd.Name != ""); err != nil {
		return true, err
	}
	if cmd.Name != "" {
		if err = s.storeTelegramMessage(ctx, botID, msg); err != nil {
			return true, err
		}
		err := s.handleRelayCommand(ctx, cfg, msg, cmd)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	if binding == nil {
		if msg.Chat.Type == models.ChatTypePrivate {
			return true, s.relayReply(ctx, botID, msg, "你尚未绑定聊天对象，请联系管理员。可发送 /id 查看用户ID。")
		}
		return true, nil
	}
	// Commands owned by other plugins remain commands, never ordinary relay content.
	if strings.HasPrefix(strings.TrimSpace(msg.Text), "/") {
		return false, nil
	}
	if err = s.storeTelegramMessage(ctx, botID, msg); err != nil {
		return true, err
	}
	if reason != "" {
		return true, s.relayReply(ctx, botID, msg, reason)
	}
	return true, nil
}

func relayMessageHasContent(msg *models.Message) bool {
	return msg.Text != "" || msg.Caption != "" || len(msg.Photo) > 0 || msg.Video != nil || msg.Animation != nil || msg.Document != nil || msg.Audio != nil || msg.Voice != nil || msg.Sticker != nil || msg.VideoNote != nil || msg.Contact != nil || msg.Location != nil || msg.Venue != nil || msg.Poll != nil || msg.Dice != nil
}

func (s *sSysBot) enqueueRelayMessage(ctx context.Context, cfg *entity.YoubanBotRelayConfig, botName string, msg *models.Message, binding *entity.YoubanBotRelayBinding, target, reason string, command bool) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return gerror.Wrap(err, "序列化聊天记录失败")
	}
	source := strconv.FormatInt(msg.Chat.ID, 10)
	pairKey := relayPairKey(source, "bot:"+strconv.FormatInt(cfg.BotId, 10))
	title := botMessageChatTitle(msg) + "-" + botName
	bindingID := int64(0)
	if binding != nil {
		pairKey = binding.PairKey
		title = binding.LeftName + "-" + binding.RightName
		bindingID = binding.Id
	}
	title = relayTopicTitle(title)
	return dao.YoubanBotRelayDelivery.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		kinds := []string{}
		if cfg.AuditDatabase == 1 {
			kinds = append(kinds, consts.RelayKindArchive)
		}
		if cfg.AuditGroupId != "" {
			kinds = append(kinds, consts.RelayKindAudit)
		}
		if binding != nil && !command && !strings.HasPrefix(strings.TrimSpace(msg.Text), "/") {
			kinds = append(kinds, consts.RelayKindChat)
		}
		for _, kind := range kinds {
			destination, status := target, consts.RelayPending
			if kind == consts.RelayKindChat && reason != "" {
				status = consts.RelayCancelled
			}
			if kind == consts.RelayKindArchive {
				destination = ""
				status = consts.RelayStored
			}
			if kind == consts.RelayKindAudit {
				destination = cfg.AuditGroupId
			}
			available := gtime.Now()
			if msg.MediaGroupID != "" {
				available = available.Add(2 * time.Second)
			}
			data := do.YoubanBotRelayDelivery{BotId: cfg.BotId, BindingId: bindingID, SourceChatId: source, SourceMessageId: msg.ID, MediaGroupId: msg.MediaGroupID, TargetChatId: destination, Kind: kind, PairKey: pairKey, TopicTitle: title, RawJson: string(raw), Status: status, AvailableAt: available, ErrorMessage: reason}
			if kind == consts.RelayKindChat && status == consts.RelayCancelled {
				data.RawJson = ""
			}
			c := dao.YoubanBotRelayDelivery.Columns()
			if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(data).OnConflict(c.BotId, c.SourceChatId, c.SourceMessageId, c.Kind, c.TargetChatId).InsertIgnore(); err != nil {
				return gerror.Wrap(err, "保存聊天投递任务失败")
			}
		}
		if binding != nil && !command && reason == "" && !strings.HasPrefix(strings.TrimSpace(msg.Text), "/") {
			if err := s.enqueueRelayFirstUse(ctx, cfg, msg, binding); err != nil {
				return err
			}
		}
		return nil
	})
}

func relayTopicTitle(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 128 {
		runes = runes[:128]
	}
	if len(runes) == 0 {
		return "双向会话"
	}
	return string(runes)
}

func (s *sSysBot) AdminRelayRecordList(ctx context.Context, in *sysin.RelayListInp) ([]*entity.YoubanBotRelayDelivery, int, error) {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return nil, 0, err
	}
	if err := in.Filter(); err != nil {
		return nil, 0, err
	}
	list := make([]*entity.YoubanBotRelayDelivery, 0)
	total := 0
	c := dao.YoubanBotRelayDelivery.Columns()
	mod := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.BotId, in.BotId)
	if in.Kind != "" {
		mod = mod.Where(c.Kind, in.Kind)
	}
	err := mod.OrderDesc(c.Id).Page(in.Page, in.PerPage).ScanAndCount(&list, &total, false)
	return list, total, gerror.Wrap(err, "读取聊天审计记录失败")
}
