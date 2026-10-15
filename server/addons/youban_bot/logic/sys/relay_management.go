package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

// Management has its own main-Bot command; it never registers relay /help globally.
type relayManagementFeature struct{}

func (relayManagementFeature) Key() string         { return "relay_management" }
func (relayManagementFeature) Command() string     { return "relayadmin" }
func (relayManagementFeature) Description() string { return "双向机器人管理" }
func (relayManagementFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (handled bool, resultErr error) {
	defer func() {
		if resultErr != nil && fc.Msg != nil {
			resultErr = s.relayInteractionError(ctx, fc.BotId, fc.Msg, resultErr)
		}
	}()
	if fc.Msg == nil || fc.Msg.From == nil {
		return true, nil
	}
	reply := func(text string) (bool, error) { return true, s.relayManagementReply(ctx, fc.BotId, fc.Msg, text) }
	if fc.Msg.Chat.Type != models.ChatTypePrivate {
		return reply("请在机器人私聊中使用双向机器人管理。")
	}
	ctx = relayActorContext(ctx, fc.Msg.From.ID)
	account, accountErr := s.relayAccount(ctx)
	if accountErr != nil {
		return reply("请先绑定上架端租户账户后使用双向机器人管理。发送 /bind 完成绑定。")
	}
	args := strings.Fields(fc.Args)
	if len(args) > 0 && args[0] == "绑定后台Bot" {
		if len(args) != 2 {
			return reply("请从自己的后台Bot列表选择。")
		}
		sourceID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return reply("Bot ID不正确")
		}
		id, err := s.relayBindBackendBot(ctx, account, sourceID)
		if err != nil {
			return reply(html.EscapeString(err.Error()))
		}
		return reply(fmt.Sprintf("已绑定当前账户在后台创建的Bot，默认停用。\n管理：<code>/relayadmin %d</code>", id))
	}
	if len(args) > 0 && args[0] == "创建" {
		return reply("这里不支持创建Bot。请先在后台创建双向机器人，再返回 /relayadmin 绑定自己的Bot。")
	}

	if page, ok := relayManagementPage(args); ok {
		if page < 1 {
			return reply("页码不正确，请返回 /relayadmin")
		}
		text, err := s.relayManagementListPage(ctx, fc.Msg.From.ID, page)
		if err != nil {
			return true, err
		}
		return reply(text)
	}
	if len(args) == 0 {
		text, err := s.relayManagementList(ctx, fc.Msg.From.ID)
		if err != nil {
			return true, err
		}
		return reply(text)
	}
	botID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || botID <= 0 {
		return reply("格式：<code>/relayadmin BotID</code>，先发送 <code>/relayadmin</code> 查看列表。")
	}
	cfg, err := relayConfig(ctx, botID)
	if err != nil {
		return true, err
	}
	if cfg == nil {
		return reply("无权管理该双向机器人，或该机器人尚未配置。")
	}
	if !relayOwned(cfg) || cfg.TenantId != account.TenantId || cfg.AccountId != account.AccountId {
		return reply("无权管理其他租户账户的双向机器人；旧全局配置需管理员迁移。")
	}
	allowed, err := s.relayIsAdmin(ctx, cfg, fc.Msg.From.ID)
	if err != nil {
		return true, err
	}
	if !allowed {
		return reply("仅该双向机器人配置的管理员可以管理。")
	}
	command := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(fc.Args), args[0]))
	if command == "" {
		state := "停用"
		if cfg.Enabled == 1 {
			state = "启用"
		}
		prefix := fmt.Sprintf("/relayadmin %d", botID)
		_, runtimeState, err := s.relayRuntimeState(ctx, cfg)
		if err != nil {
			return true, err
		}
		lines := []string{fmt.Sprintf("<b>双向机器人 #%d</b> · %s\n实际状态：%s", botID, state, html.EscapeString(runtimeState))}
		for _, action := range []string{"启用", "停用", "当前设置", "绑定列表", "屏蔽列表", "黑名单", "屏蔽跳转 开启", "屏蔽跳转 关闭", "绑定", "解绑", "添加管理员", "删除管理员", "管理员列表", "屏蔽关键词", "删除关键词", "屏蔽用户", "解除屏蔽用户", "审计设置", "聊天存档 开启", "聊天存档 关闭", "转发群", "关闭转发", "同步后台Bot"} {
			lines = append(lines, "<code>"+prefix+" "+action+"</code>")
		}
		lines = append(lines, "绑定和屏蔽命令可直接追加，例如：", "<code>"+prefix+" 绑定 @alice @bob</code>", "<code>"+prefix+" 屏蔽 关键词</code>", "使用审计设置、聊天存档和转发群按钮配置聊天记录管理。", "返回：<code>/relayadmin</code>")
		return reply(strings.Join(lines, "\n"))
	}
	if command == "同步后台Bot" {
		if err := s.relaySyncBackendBot(ctx, account, cfg); err != nil {
			return reply(html.EscapeString(err.Error()))
		}
		return reply(fmt.Sprintf("已同步后台Bot信息。返回：<code>/relayadmin %d</code>", botID))
	}
	if err := s.relayCheckAccess(ctx, cfg.BotId); err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	if prompt := relayManagementActionPrompt(botID, command); prompt != "" {
		return reply(prompt)
	}
	if command == "启用" || command == "停用" || strings.HasPrefix(command, "解绑 ") || command == "关闭转发" {
		return reply("请确认操作：\n<code>" + fmt.Sprintf("/relayadmin %d 确认 ", botID) + html.EscapeString(command) + "</code>\n取消：<code>" + fmt.Sprintf("/relayadmin %d", botID) + "</code>")
	}
	command = strings.TrimPrefix(command, "确认 ")
	var result string
	err = relayMutate(ctx, botID, func(ctx context.Context) error {
		current, err := relayConfig(ctx, botID)
		if err != nil {
			return err
		}
		if current == nil {
			return gerror.New("双向机器人配置已移除")
		}
		allowed, err := s.relayIsAdmin(ctx, current, fc.Msg.From.ID)
		if err != nil {
			return err
		}
		if !allowed {
			return gerror.New("管理员权限已变化，请重新打开管理菜单")
		}
		if command == "启用" || command == "停用" {
			enabled := 0
			if command == "启用" {
				enabled = 1
			}
			_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(current.Id).Data(do.YoubanBotRelayConfig{Enabled: enabled}).Update()
			_, state, stateErr := s.relayRuntimeState(ctx, &entity.YoubanBotRelayConfig{BotId: current.BotId, Enabled: enabled})
			if stateErr != nil {
				return stateErr
			}
			result = "双向功能开关已" + command + "。\n实际状态：" + state
			return err
		}
		if handled, text, auditErr := s.relayManagementAudit(ctx, current, command); handled {
			result = text
			return auditErr
		}
		cmd := parseRelayCommand(command)
		if cmd.Name == "" || strings.HasPrefix(cmd.Name, "/") {
			return gerror.New("未识别的管理操作，请发送 /relayadmin BotID 查看菜单")
		}
		if cmd.Name == "解绑" {
			cmd.Name = "确认解绑"
		}
		result, err = s.executeRelayCommand(ctx, current, cmd)
		return err
	})
	if err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	fields := strings.Fields(command)
	if len(fields) > 0 && (fields[0] == "绑定列表" || fields[0] == "屏蔽列表" || fields[0] == "黑名单") {
		page := 1
		if len(fields) > 1 {
			page, _ = strconv.Atoi(fields[1])
		}
		if page > 1 {
			result += fmt.Sprintf("\n上一页：<code>/relayadmin %d %s %d</code>", botID, fields[0], page-1)
		}
		result += fmt.Sprintf("\n下一页：<code>/relayadmin %d %s %d</code>", botID, fields[0], page+1)
	}
	return reply(result + fmt.Sprintf("\n\n返回：<code>/relayadmin %d</code>", botID))
}

func (s *sSysBot) relayManagementList(ctx context.Context, userID int64) (string, error) {
	return s.relayManagementListPage(ctx, userID, 1)
}
func (s *sSysBot) relayManagementListPage(ctx context.Context, userID int64, page int) (string, error) {
	account, err := s.relayBoundAccount(ctx, userID)
	if err != nil {
		return "", gerror.New("请先绑定租户账户")
	}
	var configs []*entity.YoubanBotRelayConfig
	if err := relayAccountConfigQuery(ctx, account).OrderAsc(dao.YoubanBotRelayConfig.Columns().BotId).Scan(&configs); err != nil {
		return "", gerror.Wrap(err, "读取当前账户的双向配置失败")
	}
	lines := []string{fmt.Sprintf("<b>双向机器人管理</b> · 第%d页", page)}
	visible := 0
	for _, cfg := range configs {
		if !relayOwned(cfg) || cfg.TenantId != account.TenantId || cfg.AccountId != account.AccountId {
			continue
		}
		allowed, err := s.relayIsAdmin(ctx, cfg, userID)
		if err != nil {
			return "", err
		}
		if !allowed {
			continue
		}
		visible++
		if visible <= (page-1)*8 || visible > page*8 {
			continue
		}
		bot, err := s.botById(ctx, cfg.BotId)
		if err != nil {
			return "", err
		}
		state := "停用"
		if cfg.Enabled == 1 {
			state = "启用"
		}
		_, runtimeState, err := s.relayRuntimeState(ctx, cfg)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s @%s · 开关%s · %s\n<code>/relayadmin %d</code>", html.EscapeString(bot.BotName), html.EscapeString(bot.BotUsername), state, html.EscapeString(runtimeState), cfg.BotId))
	}
	if page > 1 {
		lines = append(lines, fmt.Sprintf("上一页：<code>/relayadmin 列表 %d</code>", page-1))
	}
	if visible > page*8 {
		lines = append(lines, fmt.Sprintf("下一页：<code>/relayadmin 列表 %d</code>", page+1))
	}
	if visible == 0 {
		choices, err := s.relayBackendBotChoices(ctx, account)
		if err != nil {
			return "", err
		}
		lines = append(lines, choices)
	}
	if page > 1 {
		lines = append(lines, "返回首页：<code>/relayadmin</code>")
	}
	return strings.Join(lines, "\n\n"), nil
}
