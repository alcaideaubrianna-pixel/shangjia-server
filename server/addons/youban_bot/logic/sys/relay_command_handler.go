package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) handleRelayCommand(ctx context.Context, cfg *entity.YoubanBotRelayConfig, msg *models.Message, cmd relayCommand) error {
	userID := int64(0)
	if msg.From != nil {
		userID = msg.From.ID
	}
	if cmd.Name == "/id" {
		return s.relayReply(ctx, cfg.BotId, msg, fmt.Sprintf("用户ID：<code>%d</code>\n聊天ID：<code>%d</code>", userID, msg.Chat.ID))
	}
	admin, err := s.relayIsAdmin(ctx, cfg, userID)
	if err != nil {
		return err
	}
	if cmd.Name == "/help" {
		if !admin {
			return s.relayReply(ctx, cfg.BotId, msg, "仅配置的管理员可以查看双向机器人帮助。")
		}
		if msg.Chat.Type != models.ChatTypePrivate {
			return s.relayReply(ctx, cfg.BotId, msg, "请在机器人私聊中查看管理帮助。")
		}
		return s.relayReply(ctx, cfg.BotId, msg, relayAdminHelp)
	}

	if !admin {
		return s.relayReply(ctx, cfg.BotId, msg, "仅配置的管理员可以执行此命令。")
	}
	if msg.Chat.Type != models.ChatTypePrivate {
		return s.relayReply(ctx, cfg.BotId, msg, "请在机器人私聊中执行管理命令。")
	}
	var result string
	err = relayMutate(ctx, cfg.BotId, func(ctx context.Context) error {
		var execErr error
		result, execErr = s.executeRelayCommand(ctx, cfg, cmd)
		return execErr
	})
	if err != nil {
		return s.relayReply(ctx, cfg.BotId, msg, html.EscapeString(err.Error()))
	}
	return s.relayReply(ctx, cfg.BotId, msg, result)
}

func (s *sSysBot) relayReply(ctx context.Context, botID int64, msg *models.Message, text string) error {
	row, err := s.botById(ctx, botID)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: text, ParseMode: models.ParseModeHTML, ReplyParameters: &models.ReplyParameters{MessageID: msg.ID, AllowSendingWithoutReply: true}})
	return gerror.Wrap(err, "回复双向机器人消息失败")
}

func (s *sSysBot) executeRelayCommand(ctx context.Context, cfg *entity.YoubanBotRelayConfig, cmd relayCommand) (string, error) {
	base := sysin.RelayBotInp{BotId: cfg.BotId}
	switch cmd.Name {
	case "绑定":
		left, right, err := relayBindArguments(cmd.Args)
		if err != nil {
			return "", err
		}
		if err = s.AdminRelayBind(ctx, &sysin.RelayBindingInp{RelayBotInp: base, Left: left, Right: right}); err != nil {
			return "", err
		}
		return "双向绑定成功。", nil
	case "解绑":
		peer, err := s.relayResolvePeer(ctx, cfg.BotId, cmd.Args)
		if err != nil {
			return "", err
		}
		binding, err := relayBindingByChat(ctx, cfg.BotId, peer.ID)
		if err != nil {
			return "", err
		}
		if binding == nil {
			return "该对象没有绑定。", nil
		}
		err = s.AdminRelayUnbind(ctx, &sysin.RelayDeleteInp{RelayBotInp: base, Id: binding.Id})
		return "已解除双向绑定。", err
	case "屏蔽跳转":
		enabled := 0
		if cmd.Args == "开启" {
			enabled = 1
		} else if cmd.Args != "关闭" {
			return "", gerror.New("格式：屏蔽跳转 开启 或 屏蔽跳转 关闭")
		}
		_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{BlockLinks: enabled}).Update()
		return "屏蔽跳转已" + cmd.Args + "。", err
	case "当前设置":
		admins, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(dao.YoubanBotRelayAdmin.Columns().BotId, cfg.BotId).Count()
		if err != nil {
			return "", err
		}
		bindings, err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(dao.YoubanBotRelayBinding.Columns().BotId, cfg.BotId).Count()
		if err != nil {
			return "", err
		}
		jump := "关闭"
		if cfg.BlockLinks == 1 {
			jump = "开启"
		}
		state := "停用"
		if cfg.Enabled == 1 {
			state = "启用"
		}
		return fmt.Sprintf("双向功能：%s\n屏蔽跳转：%s\n附加管理员：%d\n双向绑定：%d", state, jump, admins, bindings), nil
	case "绑定列表", "屏蔽列表", "黑名单":
		return s.relayCommandList(ctx, cfg.BotId, cmd)
	}
	kind, value, remove := consts.RelayRuleKeyword, cmd.Args, false
	switch cmd.Name {
	case "屏蔽用户":
		kind = consts.RelayRuleUser
	case "屏蔽群":
		kind = consts.RelayRuleGroup
	case "解除屏蔽用户":
		kind = consts.RelayRuleUser
		remove = true
	case "解除屏蔽群":
		kind = consts.RelayRuleGroup
		remove = true
	case "解除屏蔽":
		kind = consts.RelayRuleUser
		remove = true
	case "删除屏蔽":
		remove = true
	case "屏蔽":
		if strings.HasPrefix(value, "@") {
			kind = consts.RelayRuleUser
		}
	default:
		return "", gerror.New("未识别的管理命令")
	}
	if !remove {
		err := s.AdminRelaySaveRule(ctx, &sysin.RelayRuleInp{RelayBotInp: base, Kind: kind, Value: value})
		return "已添加屏蔽规则。", err
	}
	if value == "" {
		return "", gerror.New("请输入需要解除的屏蔽内容")
	}
	if kind != consts.RelayRuleKeyword && strings.HasPrefix(value, "@") {
		peer, err := s.relayResolvePeer(ctx, cfg.BotId, value)
		if err != nil {
			return "", err
		}
		if peer.Kind != kind {
			return "", gerror.New("屏蔽对象类型不匹配")
		}
		value = peer.ID
	}
	_, err := dao.YoubanBotRelayRule.Ctx(ctx).Where(do.YoubanBotRelayRule{BotId: cfg.BotId, Kind: kind, Value: strings.ToLower(value)}).Delete()
	return "已移除屏蔽规则。", err
}

func (s *sSysBot) relayCommandList(ctx context.Context, botID int64, cmd relayCommand) (string, error) {
	page := 1
	if cmd.Args != "" {
		p, err := strconv.Atoi(cmd.Args)
		if err != nil || p < 1 {
			return "", gerror.New("页码必须是正整数")
		}
		page = p
	}
	in := &sysin.RelayListInp{BotId: botID}
	in.Page = page
	in.PerPage = 10
	lines := []string{}
	total := 0
	if cmd.Name == "绑定列表" {
		rows, n, err := s.AdminRelayBindingList(ctx, in)
		if err != nil {
			return "", err
		}
		total = n
		for _, r := range rows {
			lines = append(lines, fmt.Sprintf("%s ↔ %s\n<code>%s</code> ↔ <code>%s</code>", html.EscapeString(r.LeftName), html.EscapeString(r.RightName), r.LeftChatId, r.RightChatId))
		}
	} else {
		if cmd.Name == "屏蔽列表" {
			in.Kind = consts.RelayRuleKeyword
		}
		// Blacklist excludes keywords before pagination, not after it.
		var rows []*entity.YoubanBotRelayRule
		c := dao.YoubanBotRelayRule.Columns()
		mod := dao.YoubanBotRelayRule.Ctx(ctx).Where(c.BotId, botID)
		if in.Kind != "" {
			mod = mod.Where(c.Kind, in.Kind)
		} else {
			mod = mod.WhereIn(c.Kind, []string{consts.RelayRuleUser, consts.RelayRuleGroup})
		}
		if err := mod.Page(page, 10).OrderDesc(c.Id).ScanAndCount(&rows, &total, false); err != nil {
			return "", err
		}
		for _, r := range rows {
			lines = append(lines, html.EscapeString(r.Label)+" <code>"+html.EscapeString(r.Value)+"</code>")
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "暂无记录。")
	}
	return fmt.Sprintf("<b>%s</b> · 第%d页 · 共%d条\n\n%s", cmd.Name, page, total, strings.Join(lines, "\n\n")), nil
}
