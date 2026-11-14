package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/service"
)

const (
	botProfilePublishOperationPrefix   = "bot_publish:"
	botProfileUnpublishOperationPrefix = "bot_unpublish:"
)

const botProfilePublishNoticeTTL = 24 * time.Hour

func reserveBotProfilePublishNotice(ctx context.Context, profileID int64, operationNo string) (bool, error) {
	key := fmt.Sprintf("youban_publish:bot_notice:%d:%s", profileID, strings.TrimSpace(operationNo))
	return cache.Instance().SetIfNotExist(ctx, key, 1, botProfilePublishNoticeTTL)
}

type botProfilePublishNoticeTarget struct {
	BotId  int64
	ChatId string
}

type botProfilePublishNoticeRow struct {
	JobId           int64  `json:"jobId"`
	ChannelTitle    string `json:"channelTitle"`
	ChannelUsername string `json:"channelUsername"`
	TargetChatId    string `json:"targetChatId"`
	TgMessageId     int64  `json:"tgMessageId"`
	Status          string `json:"status"`
}

func newBotProfilePublishOperationNo(botId int64, chatId string, profileId int64) string {
	return newBotProfileOperationNo(botProfilePublishOperationPrefix, botId, chatId, profileId)
}

func newBotProfileUnpublishOperationNo(botId int64, chatId string, profileId int64) string {
	return newBotProfileOperationNo(botProfileUnpublishOperationPrefix, botId, chatId, profileId)
}

func newBotProfileOperationNo(prefix string, botId int64, chatId string, profileId int64) string {
	return fmt.Sprintf("%s%d:%s:%d:%d", prefix, botId, strings.TrimSpace(chatId), profileId, gtime.Now().TimestampNano())
}

func parseBotProfilePublishNoticeTarget(operationNo string) (botProfilePublishNoticeTarget, bool) {
	return parseBotProfileNoticeTarget(operationNo, botProfilePublishOperationPrefix)
}

func parseBotProfileUnpublishNoticeTarget(operationNo string) (botProfilePublishNoticeTarget, bool) {
	return parseBotProfileNoticeTarget(operationNo, botProfileUnpublishOperationPrefix)
}

func parseBotProfileNoticeTarget(operationNo, prefix string) (botProfilePublishNoticeTarget, bool) {
	parts := strings.SplitN(strings.TrimSpace(operationNo), ":", 5)
	if len(parts) != 5 || parts[0] != strings.TrimSuffix(prefix, ":") {
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
			claimed, claimErr := claimProfileOperation(ctx, capability.TenantId, profileId, profileOperationPublish)
			if claimErr != nil {
				return nil, claimErr
			}
			if !claimed {
				return nil, gerror.New("资料操作参数不完整")
			}
			operationNo := newBotProfilePublishOperationNo(botId, chatId, profileId)
			if err = s.submitProfilePublish(ctx, profileId, capability.TenantId, group.AccountId, capability.AccountId, operationNo, nil, false); err != nil {
				releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
				return nil, err
			}
			releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
			if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
				releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
				return nil, err
			}
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已提交上架"}, nil
}

func (s *sSysPublish) submitBotProfilesUnpublishWithNotice(ctx context.Context, ids []int64, capability *sysin.AccountCapabilityModel, botId int64, chatId string) (*sysin.ProfileStatusModel, error) {
	groups, err := s.profileOwnerGroups(ctx, ids, nil, capability)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		for _, profileId := range uniqueIds(group.Ids) {
			claimed, claimErr := claimProfileOperation(ctx, capability.TenantId, profileId, profileOperationUnpublish)
			if claimErr != nil {
				return nil, claimErr
			}
			if !claimed {
				return nil, gerror.New("资料操作参数不完整")
			}
			operationNo := newBotProfileUnpublishOperationNo(botId, chatId, profileId)
			if err = s.setProfilesOffline(ctx, []int64{profileId}, capability.TenantId); err != nil {
				releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
				return nil, err
			}
			if err = s.enqueueProfileDownRunWithOperationNo(ctx, capability.TenantId, []int64{profileId}, 0, operationNo); err != nil {
				releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
				return nil, gerror.Wrap(err, "加入资料下架队列失败")
			}
			releaseProfileOperationClaim(ctx, capability.TenantId, profileId)
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已提交下架"}, nil
}

func (s *sSysPublish) notifyBotProfilePublishCompleted(ctx context.Context, job telegramJobRecord) error {
	target, ok := parseBotProfilePublishNoticeTarget(job.OperationNo)
	if !ok {
		return nil
	}
	var profile struct {
		ProfileNo string      `json:"profileNo"`
		Title     string      `json:"title"`
		CreatedAt *gtime.Time `json:"createdAt"`
	}
	if err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("profile_no,title,created_at").Where("id", job.ProfileId).WhereNull("deleted_at").Scan(&profile); err != nil {
		return gerror.Wrap(err, "读取Bot上架通知资料失败")
	}
	if strings.TrimSpace(profile.ProfileNo) == "" {
		return gerror.New("Bot上架通知资料不存在")
	}
	var channels []botProfilePublishNoticeRow
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Fields("j.id AS job_id,COALESCE(NULLIF(c.channel_title,''),j.target_chat_id) AS channel_title,c.channel_username,j.target_chat_id,j.tg_message_id,j.status").
		Where("j.profile_id", job.ProfileId).Where("j.operation_no", job.OperationNo).
		OrderAsc("j.channel_id").Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取Bot上架通知频道失败")
	}
	// The message ID is persisted in the message table for media groups; do
	// not rely on a correlated subquery in the ORM projection because some
	// production drivers return the job column's zero value for that expression.
	for i := range channels {
		if channels[i].TgMessageId > 0 {
			continue
		}
		value, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("job_id", channels[i].JobId).
			Where("purpose", "display").Where("status", "sent").WhereNull("deleted_at").
			OrderAsc("tg_message_id").Value("tg_message_id")
		if err != nil {
			return gerror.Wrap(err, "读取Bot上架通知展示消息失败")
		}
		channels[i].TgMessageId = gconv.Int64(value)
	}
	text := formatBotProfilePublishNotice(profile.ProfileNo, profile.Title, profile.CreatedAt, channels)
	return botservice.SysBot().Notify(ctx, &botsysin.NotifyInp{
		BotId: target.BotId, ChatId: target.ChatId, Text: text, ParseMode: "HTML",
	})
}

func (s *sSysPublish) notifyBotProfileUnpublishCompleted(ctx context.Context, job telegramJobRecord) error {
	target, ok := parseBotProfileUnpublishNoticeTarget(job.OperationNo)
	if !ok {
		return nil
	}
	active, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).Where("operation_no", job.OperationNo).
		WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusSending, telegramJobStatusRetryWait, telegramJobStatusConfirming}).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查Bot资料下架任务状态失败")
	}
	if active > 0 {
		return nil
	}
	claimed, err := reserveBotProfilePublishNotice(ctx, job.ProfileId, job.OperationNo)
	if err != nil || !claimed {
		return err
	}
	var profile struct {
		ProfileNo string `json:"profileNo"`
		Title     string `json:"title"`
	}
	if err = g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("profile_no,title").Where("id", job.ProfileId).WhereNull("deleted_at").Scan(&profile); err != nil {
		return gerror.Wrap(err, "读取Bot下架通知资料失败")
	}
	if strings.TrimSpace(profile.ProfileNo) == "" {
		return gerror.New("Bot下架通知资料不存在")
	}
	var channels []botProfilePublishNoticeRow
	if err = g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Fields("j.id AS job_id,COALESCE(NULLIF(c.channel_title,''),j.target_chat_id) AS channel_title,c.channel_username,j.target_chat_id,j.tg_message_id,j.status").
		Where("j.profile_id", job.ProfileId).Where("j.operation_no", job.OperationNo).
		OrderAsc("j.channel_id").Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取Bot下架通知频道失败")
	}
	text := formatBotProfileUnpublishNotice(profile.ProfileNo, profile.Title, channels)
	return botservice.SysBot().Notify(ctx, &botsysin.NotifyInp{BotId: target.BotId, ChatId: target.ChatId, Text: text, ParseMode: "HTML"})
}

func formatBotProfileUnpublishNotice(profileNo, title string, channels []botProfilePublishNoticeRow) string {
	lines := []string{"<b>资料下架完成</b>", "", "编号：<code>" + html.EscapeString(strings.TrimSpace(profileNo)) + "</code>", "标题：<code>" + html.EscapeString(strings.TrimSpace(title)) + "</code>", ""}
	for _, channel := range channels {
		lines = append(lines, "["+html.EscapeString(botProfilePublishChannelLabel(channel))+"] "+botProfilePublishStatusHTML("已下架", botProfilePublishMessageURL(channel)))
	}
	return strings.Join(lines, "\n")
}

func formatBotProfilePublishNotice(profileNo string, title string, createdAt *gtime.Time, channels []botProfilePublishNoticeRow) string {
	createdText := "-"
	if createdAt != nil {
		createdText = createdAt.Format("Y-m-d H:i:s")
	}
	lines := []string{
		"<b>资料上架完成</b>", "",
		"编号：<code>" + html.EscapeString(strings.TrimSpace(profileNo)) + "</code>",
		"标题：<code>" + html.EscapeString(strings.TrimSpace(title)) + "</code>",
		"创建时间：" + html.EscapeString(createdText), "",
	}
	for _, channel := range channels {
		result := "已发布"
		if strings.TrimSpace(channel.Status) != telegramJobStatusSent {
			result = "已跳过"
		}
		lines = append(lines, "["+html.EscapeString(botProfilePublishChannelLabel(channel))+"] "+botProfilePublishStatusHTML(result, botProfilePublishMessageURL(channel)))
	}
	return strings.Join(lines, "\n")
}

func botProfilePublishChannelLabel(channel botProfilePublishNoticeRow) string {
	label := strings.TrimSpace(channel.ChannelTitle)
	if label == "" {
		label = strings.TrimSpace(channel.TargetChatId)
	}
	return label
}

func botProfilePublishMessageURL(channel botProfilePublishNoticeRow) string {
	if channel.TgMessageId <= 0 {
		return ""
	}
	username := strings.TrimPrefix(strings.TrimSpace(channel.ChannelUsername), "@")
	if username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, channel.TgMessageId)
	}
	chatId := strings.TrimPrefix(strings.TrimSpace(channel.TargetChatId), "-100")
	if chatId == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/c/%s/%d", chatId, channel.TgMessageId)
}

func botProfilePublishStatusHTML(status, messageURL string) string {
	status = html.EscapeString(strings.TrimSpace(status))
	if strings.TrimSpace(messageURL) == "" {
		return "<b>" + status + "</b>"
	}
	return `<a href="` + html.EscapeString(messageURL) + `"><b>` + status + `</b></a>`
}
