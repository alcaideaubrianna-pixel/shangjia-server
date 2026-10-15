package sys

import (
	"context"
	"strings"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) relayAuditThread(ctx context.Context, bot *tgbot.Bot, delivery *entity.YoubanBotRelayDelivery) (int, error) {
	var topic *entity.YoubanBotRelayAuditTopic
	create := false
	err := dao.YoubanBotRelayAuditTopic.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotRelayAuditTopic.Columns()
		key := do.YoubanBotRelayAuditTopic{BotId: delivery.BotId, GroupId: delivery.TargetChatId, PairKey: delivery.PairKey}
		if _, err := dao.YoubanBotRelayAuditTopic.Ctx(ctx).Data(key).OnConflict(c.BotId, c.GroupId, c.PairKey).InsertIgnore(); err != nil {
			return err
		}
		if err := dao.YoubanBotRelayAuditTopic.Ctx(ctx).Where(key).LockUpdate().Scan(&topic); err != nil {
			return err
		}
		if topic.ThreadId != 0 {
			return nil
		}
		// Commit a durable creation intent BEFORE calling Telegram. A crash or lost
		// response must not cause the next instance to create a second topic.
		_, err := dao.YoubanBotRelayAuditTopic.Ctx(ctx).WherePri(topic.Id).Data(do.YoubanBotRelayAuditTopic{ThreadId: -1}).Update()
		create = err == nil
		return err
	})
	if err != nil {
		return 0, gerror.Wrap(err, "保存审计话题创建状态失败")
	}
	if !create {
		if topic.ThreadId > 0 {
			return int(topic.ThreadId), nil
		}
		return 0, gerror.New("审计话题正在创建或创建结果不确定，请稍后重试；持续失败时请核对群话题并恢复映射")
	}
	created, createErr := bot.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{ChatID: delivery.TargetChatId, Name: delivery.TopicTitle})
	threadID := int64(-1)
	if createErr == nil && created != nil && created.MessageThreadID > 0 {
		threadID = int64(created.MessageThreadID)
	} else if createErr == nil {
		createErr = gerror.New("Telegram未返回有效的话题ID")
	}
	if createErr != nil && relaySendErrorStatus(createErr, 0) != consts.RelayUncertain {
		threadID = 0
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, saveErr := dao.YoubanBotRelayAuditTopic.Ctx(saveCtx).WherePri(topic.Id).Where(dao.YoubanBotRelayAuditTopic.Columns().ThreadId, -1).Data(do.YoubanBotRelayAuditTopic{ThreadId: threadID}).Update()
	if saveErr != nil {
		return 0, gerror.Wrap(saveErr, "保存审计话题ID失败")
	}
	return int(threadID), gerror.Wrap(createErr, "创建审计话题失败")
}
func invalidateRelayAuditThread(ctx context.Context, delivery *entity.YoubanBotRelayDelivery) error {
	_, err := dao.YoubanBotRelayAuditTopic.Ctx(ctx).Where(do.YoubanBotRelayAuditTopic{BotId: delivery.BotId, GroupId: delivery.TargetChatId, PairKey: delivery.PairKey}).Data(do.YoubanBotRelayAuditTopic{ThreadId: 0}).Update()
	return err
}
func relayTopicMissing(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "message thread not found") || strings.Contains(text, "topic not found")
}
