package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/service"
)

const botProfilePublishOperationPrefix = "bot_publish:"

type botProfilePublishNoticeTarget struct {
	BotId  int64
	ChatId string
}

type botProfilePublishNoticeRow struct {
	ChannelTitle string `json:"channelTitle"`
	Status       string `json:"status"`
}

func newBotProfilePublishOperationNo(botId int64, chatId string, profileId int64) string {
	return fmt.Sprintf("%s%d:%s:%d:%d", botProfilePublishOperationPrefix, botId, strings.TrimSpace(chatId), profileId, gtime.Now().TimestampNano())
}

func parseBotProfilePublishNoticeTarget(operationNo string) (botProfilePublishNoticeTarget, bool) {
	parts := strings.SplitN(strings.TrimSpace(operationNo), ":", 5)
	if len(parts) != 5 || parts[0] != strings.TrimSuffix(botProfilePublishOperationPrefix, ":") {
		return botProfilePublishNoticeTarget{}, false
	}
	botId, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || botId <= 0 || strings.TrimSpace(parts[2]) == "" {
		return botProfilePublishNoticeTarget{}, false
	}
	return botProfilePublishNoticeTarget{BotId: botId, ChatId: strings.TrimSpace(parts[2])}, true
}

func (s *sSysPublish) submitBotProfilesWithNotice(ctx context.Context, ids []int64, capability *sysin.AccountCapabilityModel, botId int64, chatId string) (*sysin.ProfileStatusModel, error) {
	groups, err := s.profileOwnerGroups(ctx, ids, nil, capability)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		for _, profileId := range uniqueIds(group.Ids) {
			operationNo := newBotProfilePublishOperationNo(botId, chatId, profileId)
			if err = s.submitProfilePublish(ctx, profileId, capability.TenantId, group.AccountId, capability.AccountId, operationNo, nil, false); err != nil {
				return nil, err
			}
			if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
				return nil, err
			}
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已提交上架"}, nil
}

func (s *sSysPublish) notifyBotProfilePublishCompleted(ctx context.Context, job telegramJobRecord) error {
	target, ok := parseBotProfilePublishNoticeTarget(job.OperationNo)
	if !ok {
		return nil
	}
	var profile struct {
		ProfileNo string      `json:"profileNo"`
		CreatedAt *gtime.Time `json:"createdAt"`
	}
	if err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("profile_no,created_at").Where("id", job.ProfileId).WhereNull("deleted_at").Scan(&profile); err != nil {
		return gerror.Wrap(err, "读取Bot上架通知资料失败")
	}
	if strings.TrimSpace(profile.ProfileNo) == "" {
		return gerror.New("Bot上架通知资料不存在")
	}
	var channels []botProfilePublishNoticeRow
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Fields("COALESCE(NULLIF(c.channel_title,''),j.target_chat_id) AS channel_title,j.status").
		Where("j.profile_id", job.ProfileId).Where("j.operation_no", job.OperationNo).
		OrderAsc("j.channel_id").Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取Bot上架通知频道失败")
	}
	text := formatBotProfilePublishNotice(profile.ProfileNo, profile.CreatedAt, channels)
	return botservice.SysBot().Notify(ctx, &botsysin.NotifyInp{
		BotId: target.BotId, ChatId: target.ChatId, Text: text, ParseMode: "HTML",
	})
}

func formatBotProfilePublishNotice(profileNo string, createdAt *gtime.Time, channels []botProfilePublishNoticeRow) string {
	createdText := "-"
	if createdAt != nil {
		createdText = createdAt.Format("Y-m-d H:i:s")
	}
	lines := []string{
		"<b>资料上架完成</b>", "",
		"编号：<code>" + html.EscapeString(strings.TrimSpace(profileNo)) + "</code>",
		"创建时间：" + html.EscapeString(createdText), "",
	}
	for _, channel := range channels {
		result := "已发布"
		if strings.TrimSpace(channel.Status) != telegramJobStatusSent {
			result = "已跳过"
		}
		lines = append(lines, "["+html.EscapeString(strings.TrimSpace(channel.ChannelTitle))+"] "+result)
	}
	return strings.Join(lines, "\n")
}
