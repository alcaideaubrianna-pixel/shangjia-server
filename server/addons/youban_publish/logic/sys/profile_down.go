package sys

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type profileDownPlan struct {
	Notify   bool
	Channels []telegramJobChannel
}

func (s *sSysPublish) prepareProfileDownPlan(ctx context.Context, tenantId int64) (*profileDownPlan, error) {
	res, err := NewSysConfig().PublishConfigView(ctx, &sysin.PublishConfigViewInp{})
	if err != nil {
		return nil, err
	}
	plan := &profileDownPlan{}
	if res != nil && res.PublishConfig.SkipDownChannelEnabled == 1 {
		return plan, nil
	}
	channels, err := s.telegramDownChannels(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, gerror.New("没有配置下架频道")
	}
	plan.Notify = true
	plan.Channels = channels
	return plan, nil
}

func (s *sSysPublish) handleProfilesDown(ctx context.Context, ids []int64, tenantId int64, plan *profileDownPlan) error {
	if err := s.deleteProfilesTelegramMessages(ctx, ids, tenantId); err != nil {
		return err
	}
	if plan == nil || !plan.Notify {
		return nil
	}
	return s.notifyProfilesDown(ctx, ids, tenantId, plan.Channels)
}

func (s *sSysPublish) deleteProfilesTelegramMessages(ctx context.Context, ids []int64, tenantId int64) error {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		Where("status", "sent").
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取TG下架消息失败")
	}
	for _, job := range jobs {
		if err = s.deleteTelegramJobMessagesForResubmit(ctx, job); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "deleted", "资料下架，TG历史消息已删除")
	}
	return nil
}

func (s *sSysPublish) notifyProfilesDown(ctx context.Context, ids []int64, tenantId int64, channels []telegramJobChannel) error {
	rows, err := s.profileDownRows(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	text := buildProfilesDownText(rows)
	for _, channel := range channels {
		if err = s.sendDownChannelText(ctx, tenantId, channel, text); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) telegramDownChannels(ctx context.Context, tenantId int64) ([]telegramJobChannel, error) {
	var channels []telegramJobChannel
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,bot_id_json").
		Where("tenant_id", tenantId).
		Where("publish_direction", "down").
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&channels)
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架频道失败")
	}
	return channels, nil
}

func (s *sSysPublish) profileDownRows(ctx context.Context, ids []int64, tenantId int64) ([]gdb.Record, error) {
	rows, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("profile_id,title,province,city").
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		WhereNull("deleted_at").
		OrderDesc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架资料失败")
	}
	return rows, nil
}

func buildProfilesDownText(rows []gdb.Record) string {
	lines := []string{"资料已下架"}
	for _, row := range rows {
		title := strings.TrimSpace(row["title"].String())
		region := strings.TrimSpace(row["province"].String() + " " + row["city"].String())
		if title == "" {
			title = "未命名资料"
		}
		if region != "" {
			lines = append(lines, fmt.Sprintf("- %s（%s）", title, region))
		} else {
			lines = append(lines, "- "+title)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *sSysPublish) sendDownChannelText(ctx context.Context, tenantId int64, channel telegramJobChannel, text string) error {
	chatId := normalizeTelegramChannelChatID(channel.TargetChatId)
	if chatId == "" {
		return gerror.New("下架频道Chat ID未配置")
	}
	botToken, err := s.telegramJobBotToken(ctx, firstPositiveId(decodeBotIds(channel.BotIdJson)), tenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: text})
	if err != nil {
		return gerror.Wrap(err, "推送下架频道失败")
	}
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":  tenantId,
		"action":     "down_notify",
		"status":     "success",
		"message":    fmt.Sprintf("资料下架通知已推送，频道:%d", channel.Id),
		"created_at": gtime.Now(),
	}).Insert()
	return nil
}
