package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
)

const tgTaskTypeCollectNotification = "youban_publish:collect:notification"

type collectChannelPurgeNotification struct {
	RuleOnly     bool    `json:"ruleOnly,omitempty"`
	Title        string  `json:"title"`
	ChannelID    string  `json:"channelId"`
	Link         string  `json:"link"`
	Suffix       string  `json:"suffix"`
	ProfileCount int     `json:"profileCount"`
	MessageCount int     `json:"messageCount"`
	JobIDs       []int64 `json:"jobIds"`
}

type collectNotificationPayload struct {
	PermissionCircuit *channelPermissionCircuit        `json:"permissionCircuit,omitempty"`
	Purge             *collectChannelPurgeNotification `json:"purge,omitempty"`
	ID                string                           `json:"id"`
	BotID             int64                            `json:"botId"`
	ChatID            string                           `json:"chatId,omitempty"`
	Text              string                           `json:"text,omitempty"`
	TenantID          int64                            `json:"tenantId,omitempty"`
	AccountID         int64                            `json:"accountId,omitempty"`
	SourceID          int64                            `json:"sourceId,omitempty"`
	ChannelID         string                           `json:"channelId,omitempty"`
}

func (s *sSysPublish) enqueueCollectNotification(ctx context.Context, payload collectNotificationPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectNotification, body),
		asynq.Queue(tgQueueNameBackground), asynq.TaskID("collect-notification:"+payload.ID),
		asynq.ProcessIn(time.Second), asynq.MaxRetry(20), asynq.Timeout(2*time.Minute), asynq.Retention(7*24*time.Hour))
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) handleCollectNotificationTask(ctx context.Context, task *asynq.Task) error {
	var payload collectNotificationPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "读取采集通知任务失败")
	}
	if payload.ID == "" {
		return gerror.New("采集通知任务缺少标识")
	}
	if payload.PermissionCircuit != nil {
		return s.notifyChannelPermissionCircuit(ctx, payload.PermissionCircuit)
	}
	if payload.Purge != nil {
		pending, blocked, err := s.collectChannelPurgeRemainingMessages(ctx, payload.TenantID, payload.Purge.JobIDs)
		if err != nil {
			return err
		}
		text := collectChannelPurgeProgressText(*payload.Purge, pending, blocked)
		notificationID := payload.ID
		if pending+blocked > 0 {
			notificationID += ":waiting"
		}
		if err := botService.SysBot().NotifyCollectEvent(ctx, payload.BotID, payload.ChatID, notificationID, text, nil); err != nil {
			return err
		}
		if pending+blocked > 0 {
			return gerror.New("频道资料已删除，Telegram 消息尚未全部清理，继续检查补偿结果")
		}
		return nil
	}
	if payload.SourceID <= 0 {
		return botService.SysBot().NotifyCollectEvent(ctx, payload.BotID, payload.ChatID, payload.ID, payload.Text, nil)
	}
	source, err := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("id", payload.SourceID).
		Where("tenant_id", payload.TenantID).Where("account_id", payload.AccountID).Where("bot_id", payload.BotID).
		Where("collect_enabled", 1).Where("status", 1).One()
	if err != nil {
		return err
	}
	if source.IsEmpty() {
		return nil
	}
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, payload.TenantID, 0, payload.ChannelID)
	if err != nil || blocked {
		return err
	}
	var relation struct {
		Id             int64
		CollectEnabled int
		Status         string
		ApprovalStatus string
		SourceId       int64
	}
	err = pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).As("ac").
		InnerJoin(pdao.YoubanPublishCollectChannel.Table()+" ch", "ch.id=ac.channel_id AND ch.tenant_id=ac.tenant_id").
		Fields("ac.id,ac.collect_enabled,ac.status,ac.approval_status,ac.source_id").
		Where("ac.tenant_id", payload.TenantID).Where("ac.account_id", payload.AccountID).
		WhereIn("ch.telegram_chat_id", tgChannelCacheLookupIds(payload.ChannelID)).Scan(&relation)
	if !collectOptionalRecordScanSucceeded(err) {
		return err
	}
	if relation.Id <= 0 {
		return gerror.New("新频道关系尚未创建，稍后重试通知")
	}
	if relation.SourceId != payload.SourceID || !botCollectChannelRelationEnabled(relation.CollectEnabled, relation.Status, relation.ApprovalStatus) {
		return nil
	}
	return s.sendBotCollectChannelDiscovery(ctx, source, payload.ChannelID, relation.Id, payload.ID)
}

func collectChannelPurgeProgressText(result collectChannelPurgeNotification, pending, blocked int) string {
	if pending+blocked == 0 {
		return collectChannelPurgeScopeText(collectChannelPurgeCompletedNotifyText(result.Title, result.ChannelID, result.Suffix, result.ProfileCount, result.MessageCount, len(result.JobIDs), 0, result.Link), result.RuleOnly)
	}
	cleaned := result.MessageCount - pending - blocked
	if cleaned < 0 {
		cleaned = 0
	}
	return collectChannelPurgeScopeText(fmt.Sprintf("频道代理资料已删除，消息清理未完成\n频道：%s\n频道 ID：%s%s\n原频道后缀：%s\n已删除代理资料：%d 条\n已清理推送消息：%d 条\n待清理消息：%d 条\nBOT 无法删除的消息：%d 条\n频道采集：已禁用\n状态：正在检查清理与补偿结果；无法补偿的消息需要恢复删除权限后处理。全部清理完成后会通知。", html.EscapeString(firstNonEmpty(result.Title, result.ChannelID)), html.EscapeString(result.ChannelID), collectChannelLinkLine(result.Link), html.EscapeString(result.Suffix), result.ProfileCount, cleaned, pending, blocked), result.RuleOnly)
}

func collectChannelPurgeScopeText(text string, ruleOnly bool) string {
	if !ruleOnly {
		return text
	}
	text = strings.ReplaceAll(text, "频道采集：已禁用", "当前规则的频道采集：已禁用；其他规则不受影响")
	return strings.ReplaceAll(text, "当前账户专属后缀和资料计数：已清理", "当前规则的专属后缀：已清理，历史资料归属保留")
}
