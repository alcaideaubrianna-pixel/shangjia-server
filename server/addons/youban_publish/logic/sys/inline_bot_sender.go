package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

// sendInlineTemplateByAccount is the common MTProto path used by automatic
// quick-push and template jobs. The user session sends the bot result, so
// Telegram keeps the visible "via @bot" attribution.
func (s *sSysPublish) sendInlineTemplateByAccount(ctx context.Context, tgAccountId int64, botId int64, channel *messagePushChannel, serialNo string) ([]*telegramSentMessage, error) {
	if tgAccountId <= 0 || botId <= 0 {
		return nil, gerror.New("Inline推送账号或机器人未配置")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return nil, err
	}
	var botUsername string
	if err = g.DB().Model("hg_youban_bot_bot").Safe().Ctx(ctx).Where("id", botId).Where("status", 1).WhereNull("deleted_at").Fields("bot_username").Scan(&botUsername); err != nil {
		return nil, gerror.Wrap(err, "读取推送机器人失败")
	}
	botUsername = strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	if botUsername == "" {
		return nil, gerror.New("推送机器人未配置用户名")
	}
	var sent []*telegramSentMessage
	run := func(runCtx context.Context, client *telegram.Client) error {
		resolved, resolveErr := client.API().ContactsResolveUsername(runCtx, &tg.ContactsResolveUsernameRequest{Username: botUsername})
		if resolveErr != nil {
			return resolveErr
		}
		var bot tg.InputUserClass
		for _, user := range resolved.Users {
			if item, ok := user.(*tg.User); ok {
				bot = &tg.InputUser{UserID: item.ID, AccessHash: item.AccessHash}
				break
			}
		}
		if bot == nil {
			return gerror.New("无法解析Inline机器人")
		}
		results, getErr := client.API().MessagesGetInlineBotResults(runCtx, &tg.MessagesGetInlineBotResultsRequest{Bot: bot, Peer: peer, Query: normalizeInlineTemplateSerial(serialNo), Offset: ""})
		if getErr != nil {
			return getErr
		}
		resultId := ""
		for _, item := range results.Results {
			if item != nil {
				resultId = item.GetID()
				if resultId != "" {
					break
				}
			}
		}
		if resultId == "" {
			return gerror.New("Inline机器人没有返回可发送结果")
		}
		updates, sendErr := client.API().MessagesSendInlineBotResult(runCtx, &tg.MessagesSendInlineBotResultRequest{Peer: peer, QueryID: results.QueryID, ID: resultId, ClearDraft: true, HideVia: false})
		if sendErr != nil {
			return sendErr
		}
		sent = gotdSentMessagesFromUpdates(updates, nil)
		return nil
	}
	usedRuntime, err := s.executeAccountCollectOperation(ctx, tgAccountId, 2*time.Minute, run)
	if err != nil {
		return nil, gerror.Wrap(err, fmt.Sprintf("Inline推送失败 serial:%s", serialNo))
	}
	if !usedRuntime {
		return nil, gerror.New("推送账号会话运行中不可用")
	}
	return sent, nil
}
