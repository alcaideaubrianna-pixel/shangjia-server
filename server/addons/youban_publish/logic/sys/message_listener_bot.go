package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) HandleBotMessage(ctx context.Context, in *sysin.BotMessageInp) (bool, error) {
	if in == nil {
		return false, nil
	}
	code := listenerNormalizeBindCode(in.Text)
	if !listenerBindCodeValid(code) {
		return false, nil
	}
	if err := ensureMessageListenTables(ctx); err != nil {
		return true, err
	}
	return true, s.bindListenerTargetByCode(ctx, code, in)
}

func (s *sSysPublish) bindListenerTargetByCode(ctx context.Context, code string, in *sysin.BotMessageInp) error {
	var plan listenerPlanRecord
	if err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("bind_code", code).
		WhereNull("deleted_at").
		Scan(&plan); err != nil {
		return gerror.Wrap(err, "读取监听绑定ID失败")
	}
	if plan.Id <= 0 {
		return s.replyListenerBindMessage(ctx, in.ChatId, "绑定ID不存在或已失效，请在页面重新获取。")
	}
	chatType := listenerBotChatType(in.ChatType)
	if chatType == "" {
		return s.replyListenerBindMessage(ctx, in.ChatId, "请将机器人添加到需要接收通知的群聊，并在群聊中发送绑定ID。")
	}
	currentChatId := normalizeTelegramChannelChatID(in.ChatId)
	if strings.TrimSpace(currentChatId) == "" {
		return gerror.New("当前群聊或频道无效")
	}
	if strings.TrimSpace(plan.NotifyChatId) != "" {
		return s.replyListenerBindMessage(ctx, in.ChatId, "当前监听计划已经绑定通知目标，如需更换请先在页面解绑。")
	}
	currentChatTitle := firstNonEmpty(strings.TrimSpace(in.ChatTitle), currentChatId)
	now := gtime.Now()
	_, err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("id", plan.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"notify_chat_id":    currentChatId,
			"notify_chat_type":  chatType,
			"notify_chat_title": currentChatTitle,
			"notify_bound_at":   now,
			"updated_at":        now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "绑定通知目标失败")
	}
	text := "通知目标已绑定：" + telegramEscapeText(currentChatTitle)
	return s.replyListenerBindMessage(ctx, in.ChatId, text)
}

func listenerBotChatType(chatType string) string {
	switch strings.ToLower(strings.TrimSpace(chatType)) {
	case "channel":
		return "channel"
	case "group", "supergroup":
		return "group"
	default:
		return ""
	}
}

func (s *sSysPublish) replyListenerBindMessage(ctx context.Context, chatId string, text string) error {
	chatId = normalizeTelegramChannelChatID(chatId)
	if strings.TrimSpace(chatId) == "" {
		return gerror.New("当前群聊或频道无效")
	}
	return botServiceNotify(ctx, chatId, text)
}

func botServiceNotify(ctx context.Context, chatId string, text string) error {
	return botService.SysBot().Notify(ctx, &botsysin.NotifyInp{
		ChatId:        chatId,
		Text:          text,
		ParseMode:     "HTML",
		DisableNotice: false,
	})
}
