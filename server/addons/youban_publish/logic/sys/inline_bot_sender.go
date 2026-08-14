package sys

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
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
	botUsername, err := inlineBotUsername(ctx)
	if err != nil {
		return nil, err
	}
	var sent []*telegramSentMessage
	err = s.executeTelegramAccountOperation(ctx, tgAccountId, 2*time.Minute, func(runCtx context.Context, client *telegram.Client) error {
		var sendErr error
		sent, sendErr = sendInlineTemplateWithClient(runCtx, client, peer, botUsername, serialNo)
		return sendErr
	})
	if err != nil {
		return nil, gerror.Wrap(err, fmt.Sprintf("Inline推送失败 serial:%s", serialNo))
	}
	return sent, nil
}

func inlineBotUsername(ctx context.Context) (string, error) {
	var botRow struct {
		BotUsername string `json:"bot_username"`
	}
	if err := g.DB().Model("hg_youban_bot_bot").Safe().Ctx(ctx).
		Where("is_default", 1).Where("status", 1).WhereNull("deleted_at").
		OrderAsc("id").Fields("bot_username").Scan(&botRow); err != nil {
		return "", gerror.Wrap(err, "读取推送机器人失败")
	}
	botUsername := strings.TrimPrefix(strings.TrimSpace(botRow.BotUsername), "@")
	if botUsername == "" {
		return "", gerror.New("推送机器人未配置用户名")
	}
	return botUsername, nil
}

func sendInlineTemplateWithClient(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botUsername string, serialNo string) ([]*telegramSentMessage, error) {
	if client == nil || peer == nil {
		return nil, gerror.New("Inline推送客户端或目标无效")
	}
	resolved, resolveErr := client.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: botUsername})
	if resolveErr != nil {
		return nil, resolveErr
	}
	var bot tg.InputUserClass
	for _, user := range resolved.Users {
		if item, ok := user.(*tg.User); ok {
			bot = &tg.InputUser{UserID: item.ID, AccessHash: item.AccessHash}
			break
		}
	}
	if bot == nil {
		return nil, gerror.New("无法解析Inline机器人")
	}
	results, getErr := client.API().MessagesGetInlineBotResults(ctx, &tg.MessagesGetInlineBotResultsRequest{Bot: bot, Peer: peer, Query: normalizeInlineTemplateSerial(serialNo), Offset: ""})
	if getErr != nil {
		return nil, getErr
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
		return nil, gerror.New("Inline机器人没有返回可发送结果")
	}
	randomID, randomErr := cryptorand.Int(cryptorand.Reader, big.NewInt(1<<62))
	if randomErr != nil {
		return nil, gerror.Wrap(randomErr, "生成Inline消息随机ID失败")
	}
	updates, sendErr := client.API().MessagesSendInlineBotResult(ctx, &tg.MessagesSendInlineBotResultRequest{
		Peer:       peer,
		QueryID:    results.QueryID,
		ID:         resultId,
		RandomID:   randomID.Int64() + 1,
		ClearDraft: true,
		HideVia:    false,
	})
	if sendErr != nil {
		return nil, sendErr
	}
	return gotdSentMessagesFromUpdates(updates, nil), nil
}

func (s *sSysPublish) sendInlineTemplateWithAccountFallback(ctx context.Context, tgAccountId int64, channel *messagePushChannel, template *sysin.MessageTemplateModel) ([]*telegramSentMessage, error, error) {
	if tgAccountId <= 0 || channel == nil || template == nil {
		return nil, nil, gerror.New("Inline推送参数不完整")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return nil, nil, err
	}
	botUsername, inlineSetupErr := inlineBotUsername(ctx)
	media := messageTemplateTelegramMedia(template)
	templateHash := messageTemplateHash(template)
	var source *messageTemplateForwardSource
	if !messageTemplateRequiresSanitizedUpload(media) {
		source, err = s.messageTemplateForwardSource(ctx, tgAccountId, templateHash)
		if err != nil {
			g.Log().Warningf(ctx, "读取Inline降级复用源失败 tgAccountId:%d templateHash:%s err:%+v", tgAccountId, templateHash, err)
			source = nil
		}
	}
	var sent []*telegramSentMessage
	inlineErr := inlineSetupErr
	err = s.executeTelegramAccountOperation(ctx, tgAccountId, 2*time.Minute, func(runCtx context.Context, client *telegram.Client) error {
		if inlineErr == nil {
			sent, inlineErr = sendInlineTemplateWithClient(runCtx, client, peer, botUsername, template.SerialNo)
		}
		if inlineErr == nil {
			return nil
		}
		var fallbackErr error
		sent, fallbackErr = sendMessageTemplateWithTgClient(runCtx, client, peer, telegramRichTextHTML(template.Text), media, source, tgAccountId, templateHash)
		return fallbackErr
	})
	if err != nil {
		return nil, inlineErr, err
	}
	if len(sent) == 0 {
		return nil, inlineErr, gerror.New("快速推送未返回TG消息结果")
	}
	return sent, inlineErr, nil
}
